package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;

/**
 * Все виджеты разом. Одна правка задевает несколько: отметка столпа видна и
 * в «Столпах», и меткой в «Календаре»; закрытое дело — в «Сегодня» и в
 * сроках календаря. Перечитывать всё дешевле, чем помнить, кто от чего зависит.
 */
final class Widgets {

    private Widgets() {
    }

    static void refreshAll(Context ctx) {
        Context app = ctx.getApplicationContext();
        TodayWidget.refreshAll(app);
        VowsWidget.refreshAll(app);
        ShopWidget.refreshAll(app);
        FoodWidget.refreshAll(app);
        PinsWidget.refreshAll(app);
        GoalsWidget.refreshAll(app);
        CalendarWidget.updateAll(app);
        BodyWidget.updateAll(app);
        FastWidget.updateAll(app);
    }

    /**
     * Пересобрать все виджеты целиком — небо сменилось. refreshAll
     * перечитывает только строки, а подложка и шапка задаются при сборке
     * (onUpdate), поэтому каждому виду — штатный APPWIDGET_UPDATE.
     */
    static void restyleAll(Context ctx) {
        Context app = ctx.getApplicationContext();
        AppWidgetManager mgr = AppWidgetManager.getInstance(app);
        Class<?>[] kinds = {TodayWidget.class, VowsWidget.class, ShopWidget.class, FoodWidget.class, PinsWidget.class,
                GoalsWidget.class, CalendarWidget.class, BodyWidget.class, FastWidget.class, CaptureWidget.class};
        for (Class<?> k : kinds) {
            int[] ids = mgr.getAppWidgetIds(new ComponentName(app, k));
            if (ids.length == 0) continue;
            app.sendBroadcast(new Intent(app, k).setAction(AppWidgetManager.ACTION_APPWIDGET_UPDATE)
                    .putExtra(AppWidgetManager.EXTRA_APPWIDGET_IDS, ids));
        }
    }
}
