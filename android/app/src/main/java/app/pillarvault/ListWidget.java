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
 * Каркас виджета-списка: шапка (заголовок → дашборд, «+», ↻) и строки из
 * своего RemoteViewsService. «Сегодня» устроен так же, но старше — его не
 * трогаем, чтобы не сбить уже поставленные на стол виджеты.
 */
abstract class ListWidget extends AppWidgetProvider {

    /** Разметка с id widget_title, widget_date, widget_add, widget_refresh, widget_list, widget_empty. */
    abstract int layout();

    abstract Class<?> service();

    /** Куда ведёт заголовок: путь на сайте («/#shopping»). */
    abstract String home();

    /** Окно «+» — или null, если кнопки нет. */
    abstract Intent addIntent(Context ctx);

    abstract String refreshAction();

    /** Свои номера PendingIntent: у разных виджетов они не должны совпасть. */
    abstract int requestBase();

    /** Заголовок сжатого вида — как в шапке. */
    abstract String title(Context ctx);

    /** Суть для сжатого вида (Compact) — сеть, вызывается не в главном потоке. */
    abstract Compact.Gist gist(Context ctx);

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        // сжатому виду нужны данные с сервера — в фоне, пока система ждёт (goAsync)
        final PendingResult pending = goAsync();
        final Context app = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                for (int id : ids) {
                    if (Compact.is(app, mgr, id)) {
                        mgr.updateAppWidget(id, Compact.view(app, title(app), gist(app), home(), requestBase() + 4));
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

    @Override
    public void onReceive(Context ctx, Intent intent) {
        super.onReceive(ctx, intent);
        if (refreshAction().equals(intent.getAction())) {
            refreshKind(ctx.getApplicationContext(), getClass());
        }
    }

    private RemoteViews build(Context ctx, int id) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), layout());
        Sky.header(rv, Sky.now());
        int base = requestBase();

        Intent svc = new Intent(ctx, service());
        svc.putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id);
        svc.setData(Uri.parse(svc.toUri(Intent.URI_INTENT_SCHEME))); // свой адаптер на каждый виджет
        rv.setRemoteAdapter(R.id.widget_list, svc);
        rv.setEmptyView(R.id.widget_list, R.id.widget_empty);

        PendingIntent open = PendingIntent.getActivity(ctx, base,
                new Intent(ctx, MainActivity.class).putExtra(TodayWidget.EXTRA_URL, home())
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        rv.setOnClickPendingIntent(R.id.widget_title, open);
        rv.setOnClickPendingIntent(R.id.widget_empty, open);

        Intent add = addIntent(ctx);
        if (add != null) {
            add.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
            rv.setOnClickPendingIntent(R.id.widget_add, PendingIntent.getActivity(ctx, base + 1, add,
                    PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        } else {
            rv.setViewVisibility(R.id.widget_add, android.view.View.GONE);
        }

        Intent refresh = new Intent(ctx, getClass()).setAction(refreshAction());
        rv.setOnClickPendingIntent(R.id.widget_refresh, PendingIntent.getBroadcast(ctx, base + 2, refresh,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));

        // шаблон для строк: строка дописывает действие, поэтому интент изменяемый
        int mutable = Build.VERSION.SDK_INT >= 31 ? PendingIntent.FLAG_MUTABLE : 0;
        Intent item = new Intent(ctx, WidgetAction.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        rv.setPendingIntentTemplate(R.id.widget_list, PendingIntent.getActivity(ctx, base + 3, item,
                PendingIntent.FLAG_UPDATE_CURRENT | mutable));
        return rv;
    }

    /**
     * Перечитать все виджеты этого вида. Штатным APPWIDGET_UPDATE, а не
     * только строками: у сжатого вида строк нет, суть пересобирается целиком.
     */
    static void refreshKind(Context ctx, Class<?> kind) {
        AppWidgetManager mgr = AppWidgetManager.getInstance(ctx);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(ctx, kind));
        if (ids.length > 0) {
            ctx.sendBroadcast(new Intent(ctx, kind).setAction(AppWidgetManager.ACTION_APPWIDGET_UPDATE)
                    .putExtra(AppWidgetManager.EXTRA_APPWIDGET_IDS, ids));
        }
    }
}
