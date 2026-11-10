package app.pillarvault;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.Context;
import android.content.Intent;
import android.widget.RemoteViews;

/**
 * Виджет «Захват»: пять плиток быстрого ввода — дело на сегодня, покупка,
 * запись в дневник, замер, мысль в inbox. Каждая открывает окно поверх
 * рабочего стола (WidgetAdd, BodyAdd); приложение не запускается. Данных
 * у виджета нет — сети он не ждёт, меняются только цвета неба.
 */
public class CaptureWidget extends AppWidgetProvider {

    private static final int[] TILES = {R.id.cap_task, R.id.cap_shop, R.id.cap_diary, R.id.cap_metric, R.id.cap_food, R.id.cap_thought};

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        for (int id : ids) {
            mgr.updateAppWidget(id, build(ctx));
        }
        Sky.schedule(ctx);
    }

    private static RemoteViews build(Context ctx) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_capture);
        Sky.Pal sky = Sky.now();
        rv.setInt(R.id.cap_root, "setBackgroundResource", sky.bg);
        rv.setTextColor(R.id.cap_title, sky.title);
        for (int t : TILES) {
            rv.setInt(t, "setBackgroundResource", R.drawable.widget_tile);
            rv.setTextColor(t, sky.ink);
        }
        rv.setOnClickPendingIntent(R.id.cap_task, add(ctx, 0, ""));
        rv.setOnClickPendingIntent(R.id.cap_shop, add(ctx, 1, WidgetAdd.MODE_SHOP));
        rv.setOnClickPendingIntent(R.id.cap_diary, add(ctx, 2, WidgetAdd.MODE_DIARY));
        rv.setOnClickPendingIntent(R.id.cap_thought, add(ctx, 4, WidgetAdd.MODE_THOUGHT));
        rv.setOnClickPendingIntent(R.id.cap_food, add(ctx, 6, WidgetAdd.MODE_FOOD));
        rv.setOnClickPendingIntent(R.id.cap_metric, PendingIntent.getActivity(ctx, 803,
                new Intent(ctx, BodyAdd.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        rv.setOnClickPendingIntent(R.id.cap_title, PendingIntent.getActivity(ctx, 805,
                new Intent(ctx, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        return rv;
    }

    /** Окно ввода в нужном режиме. Свой requestCode у плитки — иначе система склеит интенты. */
    private static PendingIntent add(Context ctx, int slot, String mode) {
        Intent i = new Intent(ctx, WidgetAdd.class).putExtra(WidgetAdd.EXTRA_MODE, mode)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
        return PendingIntent.getActivity(ctx, 800 + slot, i,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }
}
