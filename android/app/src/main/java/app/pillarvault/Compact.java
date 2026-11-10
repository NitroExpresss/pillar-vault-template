package app.pillarvault;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.content.res.Configuration;
import android.os.Bundle;
import android.widget.RemoteViews;

/**
 * Сжатый вид виджетов-списков: сжал до одной клетки в высоту — виджет
 * показывает суть одной строкой (число и главное), растянул — снова список.
 * Списку в такой высоте места нет: строка-другая и обрезанная шапка.
 */
final class Compact {

    /** Ниже этой высоты (dp) список не помещается — показываем суть. */
    private static final int MAX_DP = 100;

    private Compact() {
    }

    /** Суть виджета: крупное значение и строка под заголовком. */
    static final class Gist {
        final String value, sub;

        Gist(String value, String sub) {
            this.value = value;
            this.sub = sub;
        }
    }

    /** Высота виджета сейчас: в портрете — MAX_HEIGHT, в альбоме — MIN_HEIGHT. */
    static boolean is(Context ctx, AppWidgetManager mgr, int id) {
        Bundle o = mgr.getAppWidgetOptions(id);
        boolean land = ctx.getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
        int h = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT : AppWidgetManager.OPTION_APPWIDGET_MAX_HEIGHT, 0);
        return h > 0 && h < MAX_DP; // 0 — лаунчер ещё не сказал размер: считаем полным
    }

    static RemoteViews view(Context ctx, String title, Gist g, String home, int code) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_compact);
        Sky.Pal sky = Sky.now();
        rv.setInt(R.id.cmp_root, "setBackgroundResource", sky.bg);
        rv.setTextViewText(R.id.cmp_title, title);
        rv.setTextColor(R.id.cmp_title, sky.title);
        rv.setTextViewText(R.id.cmp_sub, g.sub);
        rv.setTextColor(R.id.cmp_sub, sky.soft);
        rv.setTextViewText(R.id.cmp_value, g.value);
        rv.setTextColor(R.id.cmp_value, sky.ink);
        PendingIntent open = PendingIntent.getActivity(ctx, code,
                new Intent(ctx, MainActivity.class).putExtra(TodayWidget.EXTRA_URL, home)
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        rv.setOnClickPendingIntent(R.id.cmp_root, open);
        return rv;
    }

    /** Нет входа или связи — суть честно говорит об этом. */
    static Gist trouble(boolean needLogin, String error) {
        if (needLogin) return new Gist("—", "нужен вход в приложении");
        return error != null ? new Gist("—", error) : null;
    }
}
