package app.pillarvault;

import android.app.Activity;
import android.app.PendingIntent;
import android.content.ActivityNotFoundException;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageInstaller;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * «Обновить приложение» из меню. APK лежит на самом хранилище
 * (/app/pillar-vault.apk, только по сессии): релиз приватного репозитория
 * телефону без входа в GitHub недоступен. Качаем прямо в сессию
 * PackageInstaller — без временного файла и без FileProvider, — дальше
 * система спрашивает «Обновить?» своим окном. Встанет только APK с тем же
 * ключом подписи; это проверяет система, а не мы.
 */
final class AppUpdate {

    /** Что показать на странице: state — busy, latest, done или fail. */
    interface Listener {
        void status(String state, String text);
    }

    private static final AtomicBoolean running = new AtomicBoolean();

    private AppUpdate() {
    }

    static void start(Activity act, Listener l) {
        Handler ui = new Handler(Looper.getMainLooper());
        Listener say = (state, text) -> ui.post(() -> l.status(state, text));
        if (!running.compareAndSet(false, true)) {
            say.status("busy", "Обновление уже качается…");
            return;
        }
        Context app = act.getApplicationContext();
        new Thread(() -> {
            try {
                run(act, app, say);
            } finally {
                running.set(false);
            }
        }).start();
    }

    private static void run(Activity act, Context app, Listener say) {
        say.status("busy", "Проверяю версию…");
        VaultApi.Reply r = VaultApi.get("app/version.json");
        if (r.code == 401) {
            say.status("fail", "Нужен вход в хранилище");
            return;
        }
        if (r.code == 404) {
            say.status("fail", "На сервере пока нет сборки");
            return;
        }
        if (r.code != 200) {
            say.status("fail", r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code);
            return;
        }
        int code;
        String name;
        try {
            JSONObject j = new JSONObject(r.body);
            code = j.getInt("code");
            name = j.optString("name", String.valueOf(code));
        } catch (Exception e) {
            say.status("fail", "Сервер ответил непонятно");
            return;
        }
        if (code <= BuildConfig.VERSION_CODE) {
            say.status("latest", "Стоит последняя версия " + BuildConfig.VERSION_NAME);
            return;
        }

        // «Установка неизвестных приложений» выдаётся каждому источнику
        // отдельно — без неё система отклонит установку молча
        if (!app.getPackageManager().canRequestPackageInstalls()) {
            act.runOnUiThread(() -> {
                try {
                    act.startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                            Uri.parse("package:" + app.getPackageName())));
                } catch (ActivityNotFoundException ignored) {
                }
            });
            say.status("fail", "Разреши Pillar Vault ставить приложения и нажми ещё раз");
            return;
        }

        say.status("busy", "Качаю " + name + "…");
        PackageInstaller installer = app.getPackageManager().getPackageInstaller();
        PackageInstaller.Session session = null;
        HttpURLConnection c = null;
        try {
            c = VaultApi.open("app/pillar-vault.apk");
            c.setReadTimeout(60000);
            int status = c.getResponseCode();
            VaultApi.keepCookies(c);
            if (status != 200) {
                say.status("fail", status == 401 ? "Нужен вход в хранилище" : "APK не скачался: " + status);
                return;
            }
            long size = c.getContentLengthLong();

            PackageInstaller.SessionParams params = new PackageInstaller.SessionParams(
                    PackageInstaller.SessionParams.MODE_FULL_INSTALL);
            params.setAppPackageName(app.getPackageName());
            if (size > 0) params.setSize(size);
            session = installer.openSession(installer.createSession(params));

            try (InputStream in = c.getInputStream();
                 OutputStream out = session.openWrite("pillar-vault.apk", 0, size > 0 ? size : -1)) {
                byte[] buf = new byte[64 * 1024];
                long done = 0;
                int shown = -1;
                for (int n; (n = in.read(buf)) != -1; ) {
                    out.write(buf, 0, n);
                    done += n;
                    int pct = size > 0 ? (int) (done * 100 / size) : -1;
                    if (pct >= 0 && pct / 10 != shown) {
                        shown = pct / 10;
                        say.status("busy", "Качаю " + name + "… " + pct + "%");
                    }
                }
                session.fsync(out);
            }

            Intent result = new Intent(app, Result.class);
            int mutable = Build.VERSION.SDK_INT >= 31 ? PendingIntent.FLAG_MUTABLE : 0;
            PendingIntent pi = PendingIntent.getBroadcast(app, 0, result,
                    PendingIntent.FLAG_UPDATE_CURRENT | mutable); // система дописывает в него итог
            session.commit(pi.getIntentSender());
            session.close();
            session = null;
            say.status("done", "Подтверди установку " + name);
        } catch (Exception e) {
            say.status("fail", "Обновление не скачалось: " + (e.getMessage() != null ? e.getMessage() : "нет связи"));
        } finally {
            if (session != null) session.abandon();
            if (c != null) c.disconnect();
        }
    }

    /** Итог установки от системы: окно подтверждения или причина отказа. */
    public static class Result extends BroadcastReceiver {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            int status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE);
            if (status == PackageInstaller.STATUS_PENDING_USER_ACTION) {
                Intent confirm = intent.getParcelableExtra(Intent.EXTRA_INTENT);
                if (confirm != null) {
                    confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                    ctx.startActivity(confirm);
                }
                return;
            }
            if (status == PackageInstaller.STATUS_SUCCESS) return; // процесс уже заменён новой версией
            String why;
            switch (status) {
                case PackageInstaller.STATUS_FAILURE_ABORTED:
                    why = "Обновление отменено";
                    break;
                case PackageInstaller.STATUS_FAILURE_CONFLICT:
                case PackageInstaller.STATUS_FAILURE_INCOMPATIBLE:
                    why = "Обновление не встало: другой ключ подписи — удали приложение и поставь заново";
                    break;
                case PackageInstaller.STATUS_FAILURE_STORAGE:
                    why = "Обновление не встало: мало места";
                    break;
                default:
                    String msg = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE);
                    why = "Обновление не встало" + (msg != null ? ": " + msg : "");
            }
            Toast.makeText(ctx, why, Toast.LENGTH_LONG).show();
        }
    }
}
