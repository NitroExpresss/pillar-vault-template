package app.pillarvault;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.widget.RemoteViews;

/**
 * Виджет «Сегодня» на рабочем столе — две вкладки: «Сегодня» (дела дня) и
 * «Что горит» (сроки зонами). Квадрат закрывает дело, «→» — на завтра,
 * текст открывает карточку в приложении, ↻ — обновить. Выбранная вкладка
 * помнится у каждого виджета своя. Сессию берёт у WebView приложения.
 */
public class TodayWidget extends AppWidgetProvider {

    static final String ACTION_REFRESH = "app.pillarvault.WIDGET_REFRESH";
    static final String EXTRA_DO = "do";     // done | open
    static final String EXTRA_TEXT = "text";
    static final String EXTRA_URL = "url";

    static final String ACTION_TAB = "app.pillarvault.TODAY_TAB";
    static final String EXTRA_TAB = "tab";
    static final String TAB_TODAY = "today", TAB_BURN = "burn";
    private static final String PREFS = "today_widget";

    /** Какая вкладка открыта у этого виджета. */
    static String tab(Context ctx, int id) {
        String t = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString("tab" + id, TAB_TODAY);
        return TAB_BURN.equals(t) ? TAB_BURN : TAB_TODAY;
    }

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        // сжатому виду (Compact) нужны дела с сервера — в фоне, пока система ждёт
        final PendingResult pending = goAsync();
        final Context app = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                for (int id : ids) {
                    if (Compact.is(app, mgr, id)) {
                        boolean burn = TAB_BURN.equals(tab(app, id));
                        mgr.updateAppWidget(id, Compact.view(app,
                                app.getString(burn ? R.string.widget_burn_title : R.string.widget_title),
                                burn ? burnGist() : gist(app), "/", 9));
                    } else {
                        mgr.updateAppWidget(id, build(app, id));
                        mgr.notifyAppWidgetViewDataChanged(new int[]{id}, R.id.widget_list);
                    }
                }
            } finally {
                pending.finish();
            }
        }).start();
        Sky.schedule(ctx);
    }

    @Override
    public void onAppWidgetOptionsChanged(Context ctx, AppWidgetManager mgr, int id, android.os.Bundle opts) {
        onUpdate(ctx, mgr, new int[]{id}); // растянули или сжали — список или суть
    }

    /** Суть дня одной строкой: сколько дел и первое из них. */
    private static Compact.Gist gist(Context ctx) {
        VaultApi.Today t = VaultApi.today(ctx);
        Compact.Gist bad = Compact.trouble(t.needLogin, t.error);
        if (bad != null) return bad;
        if (t.tasks.isEmpty()) return new Compact.Gist("✓", "на сегодня дел нет");
        return new Compact.Gist(String.valueOf(t.tasks.size()), t.tasks.get(0).text);
    }

    /** Суть «Что горит»: сколько просрочено или на сегодня и первое дело. */
    private static Compact.Gist burnGist() {
        VaultApi.Burn b = VaultApi.burn();
        Compact.Gist bad = Compact.trouble(b.needLogin, b.error);
        if (bad != null) return bad;
        int over = 0, now = 0;
        VaultApi.Due first = null;
        for (VaultApi.Due d : b.items) {
            if ("over".equals(d.zone)) over++;
            if ("now".equals(d.zone)) now++;
            if (first == null) first = d;
        }
        if (first == null) return new Compact.Gist("✓", "ничего не горит");
        if (over > 0) return new Compact.Gist(String.valueOf(over), "просрочено: " + first.text);
        if (now > 0) return new Compact.Gist(String.valueOf(now), "сегодня: " + first.text);
        return new Compact.Gist("✓", "дальше: " + first.text + " — " + first.when);
    }

    private static RemoteViews build(Context ctx, int id) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today);
        Sky.Pal sky = Sky.now();
        Sky.header(rv, sky);
        // вкладки: выбранная — на стеклянной плитке и цветом заголовка, другая — приглушена
        boolean burn = TAB_BURN.equals(tab(ctx, id));
        int tile = R.drawable.widget_tile;
        rv.setInt(R.id.widget_title, "setBackgroundResource", burn ? 0 : tile);
        rv.setInt(R.id.widget_tab2, "setBackgroundResource", burn ? tile : 0);
        rv.setTextColor(R.id.widget_title, burn ? sky.soft : sky.title);
        rv.setTextColor(R.id.widget_tab2, burn ? sky.title : sky.soft);
        rv.setOnClickPendingIntent(R.id.widget_title, tabIntent(ctx, id, TAB_TODAY));
        rv.setOnClickPendingIntent(R.id.widget_tab2, tabIntent(ctx, id, TAB_BURN));
        rv.setViewVisibility(R.id.widget_add, burn ? android.view.View.GONE : android.view.View.VISIBLE);

        Intent svc = new Intent(ctx, TodayWidgetService.class);
        svc.putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id);
        svc.setData(Uri.parse(svc.toUri(Intent.URI_INTENT_SCHEME))); // свой адаптер на каждый виджет
        rv.setRemoteAdapter(R.id.widget_list, svc);
        rv.setEmptyView(R.id.widget_list, R.id.widget_empty);

        // пустой список — открыть дашборд (заголовок теперь — вкладка)
        PendingIntent open = PendingIntent.getActivity(ctx, 0,
                new Intent(ctx, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        rv.setOnClickPendingIntent(R.id.widget_empty, open);

        // «+» — окно добавления дела поверх рабочего стола
        Intent add = new Intent(ctx, WidgetAdd.class)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
        rv.setOnClickPendingIntent(R.id.widget_add, PendingIntent.getActivity(ctx, 3, add,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));

        Intent refresh = new Intent(ctx, TodayWidget.class).setAction(ACTION_REFRESH);
        rv.setOnClickPendingIntent(R.id.widget_refresh, PendingIntent.getBroadcast(ctx, 1, refresh,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));

        // шаблон для строк списка: строка дописывает действие и текст дела,
        // поэтому интент обязан быть изменяемым; ведёт в невидимую WidgetAction
        int mutable = Build.VERSION.SDK_INT >= 31 ? PendingIntent.FLAG_MUTABLE : 0;
        Intent item = new Intent(ctx, WidgetAction.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        rv.setPendingIntentTemplate(R.id.widget_list, PendingIntent.getActivity(ctx, 2, item,
                PendingIntent.FLAG_UPDATE_CURRENT | mutable));
        return rv;
    }

    /** Нажатие на вкладку — свой requestCode у каждого виджета и вкладки. */
    private static PendingIntent tabIntent(Context ctx, int id, String tab) {
        Intent i = new Intent(ctx, TodayWidget.class).setAction(ACTION_TAB)
                .putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id).putExtra(EXTRA_TAB, tab);
        return PendingIntent.getBroadcast(ctx, 300000 + id * 2 + (TAB_BURN.equals(tab) ? 1 : 0), i,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    @Override
    public void onReceive(Context ctx, Intent intent) {
        super.onReceive(ctx, intent);
        if (ACTION_REFRESH.equals(intent.getAction())) {
            refreshAll(ctx);
        } else if (ACTION_TAB.equals(intent.getAction())) {
            int id = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID);
            if (id == AppWidgetManager.INVALID_APPWIDGET_ID) return;
            ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                    .putString("tab" + id, intent.getStringExtra(EXTRA_TAB)).apply();
            onUpdate(ctx, AppWidgetManager.getInstance(ctx), new int[]{id});
        }
    }

    @Override
    public void onDeleted(Context ctx, int[] ids) {
        android.content.SharedPreferences.Editor e = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit();
        for (int id : ids) e.remove("tab" + id);
        e.apply();
    }

    /**
     * Перечитать дела во всех виджетах — после правки в приложении или с
     * виджета. Целиком (APPWIDGET_UPDATE): у сжатого вида строк нет.
     */
    static void refreshAll(Context ctx) {
        Context app = ctx.getApplicationContext();
        AppWidgetManager mgr = AppWidgetManager.getInstance(app);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(app, TodayWidget.class));
        if (ids.length > 0) {
            app.sendBroadcast(new Intent(app, TodayWidget.class).setAction(AppWidgetManager.ACTION_APPWIDGET_UPDATE)
                    .putExtra(AppWidgetManager.EXTRA_APPWIDGET_IDS, ids));
        }
    }
}
