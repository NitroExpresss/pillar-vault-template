package app.pillarvault;

import android.app.Activity;
import android.appwidget.AppWidgetManager;
import android.content.ComponentName;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.graphics.Bitmap;
import android.net.Uri;
import android.os.Bundle;
import android.webkit.CookieManager;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Toast;

import org.json.JSONObject;

/**
 * Обёртка над веб-интерфейсом хранилища: одно окно, один WebView.
 * Вся логика — на сервере; здесь только то, чего браузерная вкладка не даёт:
 * своя иконка, живучая сессия, кнопка «назад» по истории страницы.
 */
public class MainActivity extends Activity {

    private static final String HOME = BuildConfig.VAULT_URL;
    private static final String HOST = Uri.parse(HOME).getHost();
    /** Откуда стартует приложение: главная (приложение на React отдаётся с корня). */
    private static final String START = HOME.endsWith("/") ? HOME : HOME + "/";

    private static final int PICK_FILE = 1;

    private WebView web;
    private ValueCallback<Uri[]> pendingFiles; // ждёт ответа от выбора файла
    private boolean offline;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        web = new WebView(this);
        web.setBackgroundColor(getColor(R.color.bg));
        setContentView(web);
        Refresh.apply(this); // частота кадров — как у экрана в настройках, а не 60 по умолчанию

        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);          // localStorage: тема, свёрнутое меню, порядок панелей
        s.setDatabaseEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setUserAgentString(s.getUserAgentString() + " PillarVaultApp/" + BuildConfig.VERSION_NAME);

        // Кука сессии живёт 7 дней — пусть переживает перезапуск приложения.
        CookieManager cookies = CookieManager.getInstance();
        cookies.setAcceptCookie(true);
        cookies.setAcceptThirdPartyCookies(web, false);

        web.setWebViewClient(new VaultClient());
        // <input type=file> в WebView сам ничего не открывает: выбор файла
        // (фото витрины) отдаётся системному диалогу и возвращается сюда
        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback,
                                             FileChooserParams params) {
                if (pendingFiles != null) pendingFiles.onReceiveValue(null);
                pendingFiles = callback;
                try {
                    startActivityForResult(params.createIntent(), PICK_FILE);
                } catch (ActivityNotFoundException e) {
                    pendingFiles = null;
                    callback.onReceiveValue(null);
                    return false;
                }
                return true;
            }
        });

        // Мост для страницы: кнопки «На рабочий стол» и «Обновить приложение».
        // Страница грузится только со своего хранилища (чужие ссылки уходят
        // в браузер), а мост умеет лишь попросить лаунчер добавить виджет
        // и поставить APK с того же хранилища — подпись сверяет система.
        web.addJavascriptInterface(new AppBridge(), "PillarApp");

        if (state != null) {
            web.restoreState(state);
        } else {
            web.loadUrl(target(getIntent()));
        }
    }

    private class AppBridge {
        /** kind: today, calendar, body, vows, shop, pins, capture или goals; что-то иное — «Сегодня». */
        private ComponentName widget(String kind) {
            Class<?> c = "calendar".equals(kind) ? CalendarWidget.class
                    : "body".equals(kind) ? BodyWidget.class
                    : "vows".equals(kind) ? VowsWidget.class
                    : "shop".equals(kind) ? ShopWidget.class
                    : "food".equals(kind) ? FoodWidget.class
                    : "fast".equals(kind) ? FastWidget.class
                    : "pins".equals(kind) ? PinsWidget.class
                    : "capture".equals(kind) ? CaptureWidget.class
                    : "goals".equals(kind) ? GoalsWidget.class : TodayWidget.class;
            return new ComponentName(MainActivity.this, c);
        }

        /** Какие виджеты знает это приложение — страница не предложит чужой. */
        @JavascriptInterface
        public String kinds() {
            return "today,calendar,body,vows,shop,food,fast,pins,capture,goals";
        }

        /** Сколько таких виджетов уже стоит — страница прячет кнопку, если есть. */
        @JavascriptInterface
        public int widgetCount(String kind) {
            return AppWidgetManager.getInstance(MainActivity.this).getAppWidgetIds(widget(kind)).length;
        }

        /**
         * Попросить лаунчер поставить виджет. Искать его в списке не нужно —
         * на Xiaomi сторонние виджеты спрятаны глубоко. false — лаунчер не умеет.
         */
        @JavascriptInterface
        public boolean pinWidget(String kind) {
            AppWidgetManager mgr = AppWidgetManager.getInstance(MainActivity.this);
            if (!mgr.isRequestPinAppWidgetSupported()) {
                runOnUiThread(() -> Toast.makeText(MainActivity.this,
                        "Лаунчер не умеет добавлять виджеты сам: зажми пустое место → Виджеты → Pillar Vault",
                        Toast.LENGTH_LONG).show());
                return false;
            }
            return mgr.requestPinAppWidget(widget(kind), null, null);
        }

        // без аргумента — как в версии 1.0.5, где виджет был один
        @JavascriptInterface
        public int widgetCount() {
            return widgetCount("today");
        }

        @JavascriptInterface
        public boolean pinWidget() {
            return pinWidget("today");
        }

        /** Номер сборки — страница сравнит со свежим на сервере (/app/version.json). */
        @JavascriptInterface
        public int versionCode() {
            return BuildConfig.VERSION_CODE;
        }

        @JavascriptInterface
        public String versionName() {
            return BuildConfig.VERSION_NAME;
        }

        /**
         * «Обновить приложение» из меню. Ход дела страница узнаёт через
         * window.pvAppUpdate(state, text): busy, latest, done, fail.
         */
        @JavascriptInterface
        public void update() {
            runOnUiThread(() -> AppUpdate.start(MainActivity.this, (state, text) -> {
                if (isFinishing() || isDestroyed()) return;
                web.evaluateJavascript("window.pvAppUpdate&&pvAppUpdate(" + JSONObject.quote(state)
                        + "," + JSONObject.quote(text) + ")", null);
            }));
        }
    }

    /** Куда открыться: карточка из виджета («/doc/…») или главная. */
    private static String target(Intent intent) {
        String path = intent == null ? null : intent.getStringExtra(TodayWidget.EXTRA_URL);
        if (path == null || !path.startsWith("/")) return START;
        return (HOME.endsWith("/") ? HOME.substring(0, HOME.length() - 1) : HOME) + path;
    }

    private class VaultClient extends WebViewClient {
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
            Uri uri = req.getUrl();
            String scheme = uri.getScheme();
            if (("https".equals(scheme) || "http".equals(scheme)) && sameSite(uri.getHost())) {
                return false; // своё хранилище — внутри приложения
            }
            // чужие сайты, mailto:, tel: — наружу, в системный обработчик
            try {
                startActivity(new Intent(Intent.ACTION_VIEW, uri));
            } catch (ActivityNotFoundException ignored) {
            }
            return true;
        }

        @Override
        public void onPageStarted(WebView view, String url, Bitmap favicon) {
            // Страница «недоступно» грузится без адреса и приходит сюда как
            // about:blank, а не data: — прежняя проверка сбрасывала флаг сразу,
            // и «назад» / запуск с иконки снова упирались в упавшую страницу.
            if (url.startsWith("http")) offline = false;
        }

        @Override
        public void onPageFinished(WebView view, String url) {
            CookieManager.getInstance().flush();
        }

        @Override
        public void onReceivedError(WebView view, WebResourceRequest req, WebResourceError err) {
            if (!req.isForMainFrame()) return;
            offline = true;
            view.loadDataWithBaseURL(null, offlinePage(String.valueOf(err.getDescription())),
                    "text/html", "utf-8", null);
        }
    }

    private static boolean sameSite(String host) {
        if (host == null || HOST == null) return false;
        return host.equals(HOST) || host.endsWith("." + HOST)
                || HOST.endsWith("." + host); // www.vault.example.com ↔ vault.example.com
    }

    private static String offlinePage(String why) {
        String safe = why.replace("&", "&amp;").replace("<", "&lt;");
        return "<!doctype html><meta name=viewport content='width=device-width,initial-scale=1'>"
                + "<body style='margin:0;height:100vh;display:flex;flex-direction:column;align-items:center;"
                + "justify-content:center;gap:14px;background:linear-gradient(180deg,#7fb5e0,#b7d8ef 55%,#f2e6cd);"
                + "color:#10261c;font-family:sans-serif;text-align:center;padding:24px;box-sizing:border-box'>"
                + "<div style='font-size:18px;font-weight:700'>Хранилище недоступно</div>"
                + "<div style='color:#3d5a50;font-size:13px'>" + safe + "</div>"
                + "<a href='" + START + "' style='margin-top:8px;padding:12px 24px;border-radius:14px;"
                + "background:#13794b;color:#fff;text-decoration:none;font-weight:700'>Повторить</a>"
                + "</body>";
    }

    @Override
    public void onBackPressed() {
        if (offline) {
            super.onBackPressed();
        } else if (web.canGoBack()) {
            web.goBack(); // интерфейс живёт на #/-маршрутах — назад идёт по ним
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        if (intent.hasExtra(TodayWidget.EXTRA_URL)) {
            web.loadUrl(target(intent)); // нажатие по делу в виджете — сразу его карточка
        } else if (offline) {
            web.loadUrl(START); // повторный запуск с иконки — вернуть на главную, если страница упала
        }
    }

    @Override
    protected void onSaveInstanceState(Bundle out) {
        super.onSaveInstanceState(out);
        web.saveState(out);
    }

    @Override
    protected void onResume() {
        super.onResume();
        Refresh.apply(this); // настройку частоты могли сменить, пока приложение было в фоне
        web.onResume();
    }

    @Override
    protected void onPause() {
        CookieManager.getInstance().flush(); // сессия на диск, пока процесс жив
        Widgets.refreshAll(this);            // дела, столпы, покупки могли поменяться — виджеты перечитают
        web.onPause();
        super.onPause();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == PICK_FILE && pendingFiles != null) {
            // отмена — null: страница должна узнать, что выбор закрыт, иначе
            // поле файла больше не откроется до перезагрузки
            pendingFiles.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data));
            pendingFiles = null;
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    @Override
    protected void onDestroy() {
        if (web != null) {
            web.setWebViewClient(new WebViewClient());
            web.destroy();
        }
        super.onDestroy();
    }
}
