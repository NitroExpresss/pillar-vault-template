package app.pillarvault;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.widget.Toast;

/**
 * Невидимая прокладка для нажатий по строкам виджетов. Открывать приложение
 * из BroadcastReceiver новые Android запрещают, а из активности — можно.
 * «Открыть» — переходит в приложение на карточку; остальное («выполнено»,
 * «куплено», отметка столпа) шлёт запрос в фоне и закрывается сразу —
 * приложение не всплывает. Не вышло — короткий тост с причиной.
 */
public class WidgetAction extends Activity {

    static final String EXTRA_MARK = "mark"; // ✓ или ✗ — для столпа
    static final String EXTRA_SRC = "src";   // карточка, где живёт покупка

    /** Запрос к хранилищу; бросает с причиной отказа. */
    private interface Call {
        void run() throws Exception;
    }

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        Intent in = getIntent();
        String what = in.getStringExtra(TodayWidget.EXTRA_DO);
        final String text = in.getStringExtra(TodayWidget.EXTRA_TEXT);
        final Context app = getApplicationContext();
        if ("open".equals(what)) {
            startActivity(new Intent(this, MainActivity.class)
                    .putExtra(TodayWidget.EXTRA_URL, in.getStringExtra(TodayWidget.EXTRA_URL))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        } else if ("card".equals(what)) {
            startActivity(CardView.of(this, in.getStringExtra(EXTRA_SRC)));
        } else if ("done".equals(what)) {
            inBackground(app, "Дело не закрыто", () -> VaultApi.markDone(app, text));
        } else if ("vow".equals(what)) {
            final String mark = in.getStringExtra(EXTRA_MARK);
            inBackground(app, "Отметка не записана", () -> VaultApi.post("vow", "name", text, "mark", mark));
        } else if ("taskdone".equals(what)) {
            final String src = in.getStringExtra(EXTRA_SRC);
            inBackground(app, "Дело не закрыто", () -> VaultApi.post("task", "do", "done", "src", src, "text", text));
        } else if ("move".equals(what)) {
            final String src = in.getStringExtra(EXTRA_SRC);
            inBackground(app, "Срок не перенесён", () -> VaultApi.post("task", "do", "move", "src", src, "text", text));
            toast(app, "На завтра: " + text);
        } else if ("later".equals(what)) {
            inBackground(app, "Дело не перенесено", () -> VaultApi.today("later", text));
            toast(app, "На завтра: " + text);
        } else if ("bought".equals(what)) {
            final String src = in.getStringExtra(EXTRA_SRC);
            inBackground(app, "Покупка не отмечена", () -> VaultApi.post("task",
                    "do", "done", "src", src != null ? src : "tasks/shopping", "text", text, "back", "/#shopping"));
        }
        finish();
    }

    private static void toast(Context app, String s) {
        Toast.makeText(app, s, Toast.LENGTH_SHORT).show();
    }

    private static void inBackground(Context app, String failTitle, Call call) {
        new Thread(() -> {
            try {
                call.run();
            } catch (Exception e) {
                String why = e.getMessage() != null ? e.getMessage() : "нет связи с хранилищем";
                new Handler(Looper.getMainLooper()).post(() ->
                        Toast.makeText(app, failTitle + ": " + why, Toast.LENGTH_SHORT).show());
            } finally {
                Widgets.refreshAll(app); // одна правка видна в нескольких виджетах
            }
        }).start();
    }
}
