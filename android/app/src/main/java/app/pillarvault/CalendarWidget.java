package app.pillarvault;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.text.SpannableStringBuilder;
import android.text.Spanned;
import android.text.style.ForegroundColorSpan;
import android.view.View;
import android.widget.RemoteViews;

import org.json.JSONArray;
import org.json.JSONObject;

/**
 * Виджет «Календарь»: месяц сеткой с метками хранилища — сроки, дневник,
 * замеры, столпы. ‹ › листают месяцы, ● — к текущему, день открывает
 * календарь приложения на этом дне. Данные — /calendar.json по сессии
 * WebView; последний ответ хранится, чтобы без связи сетка не пустела.
 */
public class CalendarWidget extends AppWidgetProvider {

    static final String ACTION_NAV = "app.pillarvault.CAL_NAV";
    static final String EXTRA_MONTH = "month"; // ГГГГ-ММ; пусто — текущий

    private static final String PREFS = "calendar_widget";

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        refresh(ctx, ids, goAsync());
        Sky.schedule(ctx);
    }

    @Override
    public void onReceive(Context ctx, Intent intent) {
        super.onReceive(ctx, intent);
        if (!ACTION_NAV.equals(intent.getAction())) return;
        int id = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID);
        if (id == AppWidgetManager.INVALID_APPWIDGET_ID) return;
        prefs(ctx).edit().putString("month" + id, intent.getStringExtra(EXTRA_MONTH)).apply();
        refresh(ctx, new int[]{id}, goAsync());
    }

    @Override
    public void onDeleted(Context ctx, int[] ids) {
        SharedPreferences.Editor e = prefs(ctx).edit();
        for (int id : ids) e.remove("month" + id).remove("json" + id);
        e.apply();
    }

    /** Перечитать все календари — после правок в приложении. */
    static void updateAll(Context ctx) {
        AppWidgetManager mgr = AppWidgetManager.getInstance(ctx);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(ctx, CalendarWidget.class));
        if (ids.length > 0) refresh(ctx.getApplicationContext(), ids, null);
    }

    /** Сеть — в отдельном потоке; pending (из goAsync) закрывается по окончании. */
    private static void refresh(Context ctx, int[] ids, PendingResult pending) {
        final Context app = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                AppWidgetManager mgr = AppWidgetManager.getInstance(app);
                for (int id : ids) {
                    mgr.updateAppWidget(id, build(app, id));
                }
            } finally {
                if (pending != null) pending.finish();
            }
        }).start();
    }

    private static SharedPreferences prefs(Context ctx) {
        return ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
    }

    private static RemoteViews build(Context ctx, int id) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_calendar);
        Sky.Pal sky = Sky.now();
        rv.setInt(R.id.cal_root, "setBackgroundResource", sky.bg);
        rv.setTextColor(R.id.cal_title, sky.title);
        rv.setTextColor(R.id.cal_prev, sky.accent);
        rv.setTextColor(R.id.cal_next, sky.accent);
        rv.setTextColor(R.id.cal_today, sky.accent);
        rv.setTextColor(R.id.cal_status, sky.warm);
        for (int i = 0; i < WEEKDAYS.length; i++) {
            rv.setTextColor(WEEKDAYS[i], i >= 5 ? sky.ember : sky.soft); // выходные — тёплым, как в приложении
        }
        SharedPreferences p = prefs(ctx);
        String month = p.getString("month" + id, "");
        if (month == null) month = "";

        VaultApi.Reply r = VaultApi.get("calendar.json" + (month.isEmpty() ? "" : "?cal=" + month));
        String status = null;
        String body = r.body;
        if (r.code == 200) {
            p.edit().putString("json" + id, body).apply();
        } else {
            status = r.code == 401 ? "Войди в приложение — виджет возьмёт ту же сессию" : "Нет связи — показан прошлый месяц из памяти";
            body = p.getString("json" + id, null);
        }
        rv.setViewVisibility(R.id.cal_status, status == null ? View.GONE : View.VISIBLE);
        if (status != null) rv.setTextViewText(R.id.cal_status, status);

        JSONObject m = null;
        try {
            if (body != null) m = new JSONObject(body);
        } catch (Exception ignored) {
        }
        if (m == null) {
            rv.setTextViewText(R.id.cal_title, "Календарь");
            rv.setOnClickPendingIntent(R.id.cal_title, openApp(ctx, id * 100 + 99, "/#calendar"));
            for (int row = 0; row < 6; row++) rv.setViewVisibility(ROWS[row], View.GONE);
            return rv;
        }

        String cur = m.optString("month");
        rv.setTextViewText(R.id.cal_title, m.optString("title"));
        rv.setOnClickPendingIntent(R.id.cal_title, openApp(ctx, id * 100 + 99, "/?cal=" + cur + "#calendar"));
        rv.setOnClickPendingIntent(R.id.cal_prev, nav(ctx, id, 1, m.optString("prev")));
        rv.setOnClickPendingIntent(R.id.cal_next, nav(ctx, id, 2, m.optString("next")));
        rv.setOnClickPendingIntent(R.id.cal_today, nav(ctx, id, 3, ""));

        JSONArray weeks = m.optJSONArray("weeks");
        for (int row = 0; row < 6; row++) {
            JSONArray wk = weeks != null ? weeks.optJSONArray(row) : null;
            rv.setViewVisibility(ROWS[row], wk == null ? View.GONE : View.VISIBLE);
            if (wk == null) continue;
            for (int col = 0; col < 7; col++) {
                JSONObject d = wk.optJSONObject(col);
                if (d == null) continue;
                int n = NUMS[row][col], mk = MARKS[row][col], cell = CELLS[row][col];
                boolean in = d.optBoolean("in"), today = d.optBoolean("today");
                rv.setTextViewText(n, String.valueOf(d.optInt("day")));
                rv.setTextColor(n, today ? sky.accent : in ? sky.ink : sky.faint);
                rv.setInt(cell, "setBackgroundResource", today ? sky.calToday : in ? sky.calDay : 0);
                rv.setTextViewText(mk, marks(d, sky));
                String date = d.optString("date");
                // день — окном поверх рабочего стола; полный календарь — заголовком
                rv.setOnClickPendingIntent(cell, PendingIntent.getActivity(ctx, 100000 + id * 100 + row * 7 + col,
                        DayView.of(ctx, date), PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
            }
        }
        return rv;
    }

    /** Метки дня цветными точками — те же цвета, что в календаре на сайте. */
    private static CharSequence marks(JSONObject d, Sky.Pal sky) {
        SpannableStringBuilder s = new SpannableStringBuilder();
        if (d.optInt("due") > 0) dot(s, "●", sky.ember);
        if (d.optBoolean("diary")) dot(s, "●", sky.line);
        if (d.optBoolean("metric")) dot(s, "●", sky.blue);
        int held = d.optInt("vowHeld"), broke = d.optInt("vowBroke");
        if (broke > 0) dot(s, "▲", sky.ember);
        else if (held > 0) dot(s, "▲", sky.accent);
        if (s.length() == 0) s.append(" "); // строка меток держит высоту клетки
        return s;
    }

    private static void dot(SpannableStringBuilder s, String glyph, int color) {
        int at = s.length();
        s.append(glyph);
        s.setSpan(new ForegroundColorSpan(color), at, s.length(), Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
    }

    private static PendingIntent nav(Context ctx, int id, int slot, String month) {
        Intent i = new Intent(ctx, CalendarWidget.class).setAction(ACTION_NAV)
                .putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id)
                .putExtra(EXTRA_MONTH, month);
        return PendingIntent.getBroadcast(ctx, id * 10 + slot, i,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private static PendingIntent openApp(Context ctx, int code, String url) {
        Intent i = new Intent(ctx, MainActivity.class)
                .putExtra(TodayWidget.EXTRA_URL, url)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        // свой requestCode у каждой клетки: иначе система склеит интенты и
        // все дни откроют один и тот же
        return PendingIntent.getActivity(ctx, 100000 + code, i,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    // id клеток из widget_calendar.xml: cal_c{строка}{столбец} — клетка,
    // cal_n — число, cal_m — метки. Массивы, а не getIdentifier: опечатку
    // поймает компилятор, а не пустая клетка на рабочем столе.
    private static final int[] WEEKDAYS = {R.id.cal_wd0, R.id.cal_wd1, R.id.cal_wd2, R.id.cal_wd3, R.id.cal_wd4, R.id.cal_wd5, R.id.cal_wd6};
    private static final int[] ROWS = {R.id.cal_row0, R.id.cal_row1, R.id.cal_row2, R.id.cal_row3, R.id.cal_row4, R.id.cal_row5};
    private static final int[][] CELLS = {
            {R.id.cal_c00, R.id.cal_c01, R.id.cal_c02, R.id.cal_c03, R.id.cal_c04, R.id.cal_c05, R.id.cal_c06},
            {R.id.cal_c10, R.id.cal_c11, R.id.cal_c12, R.id.cal_c13, R.id.cal_c14, R.id.cal_c15, R.id.cal_c16},
            {R.id.cal_c20, R.id.cal_c21, R.id.cal_c22, R.id.cal_c23, R.id.cal_c24, R.id.cal_c25, R.id.cal_c26},
            {R.id.cal_c30, R.id.cal_c31, R.id.cal_c32, R.id.cal_c33, R.id.cal_c34, R.id.cal_c35, R.id.cal_c36},
            {R.id.cal_c40, R.id.cal_c41, R.id.cal_c42, R.id.cal_c43, R.id.cal_c44, R.id.cal_c45, R.id.cal_c46},
            {R.id.cal_c50, R.id.cal_c51, R.id.cal_c52, R.id.cal_c53, R.id.cal_c54, R.id.cal_c55, R.id.cal_c56}
    };
    private static final int[][] NUMS = {
            {R.id.cal_n00, R.id.cal_n01, R.id.cal_n02, R.id.cal_n03, R.id.cal_n04, R.id.cal_n05, R.id.cal_n06},
            {R.id.cal_n10, R.id.cal_n11, R.id.cal_n12, R.id.cal_n13, R.id.cal_n14, R.id.cal_n15, R.id.cal_n16},
            {R.id.cal_n20, R.id.cal_n21, R.id.cal_n22, R.id.cal_n23, R.id.cal_n24, R.id.cal_n25, R.id.cal_n26},
            {R.id.cal_n30, R.id.cal_n31, R.id.cal_n32, R.id.cal_n33, R.id.cal_n34, R.id.cal_n35, R.id.cal_n36},
            {R.id.cal_n40, R.id.cal_n41, R.id.cal_n42, R.id.cal_n43, R.id.cal_n44, R.id.cal_n45, R.id.cal_n46},
            {R.id.cal_n50, R.id.cal_n51, R.id.cal_n52, R.id.cal_n53, R.id.cal_n54, R.id.cal_n55, R.id.cal_n56}
    };
    private static final int[][] MARKS = {
            {R.id.cal_m00, R.id.cal_m01, R.id.cal_m02, R.id.cal_m03, R.id.cal_m04, R.id.cal_m05, R.id.cal_m06},
            {R.id.cal_m10, R.id.cal_m11, R.id.cal_m12, R.id.cal_m13, R.id.cal_m14, R.id.cal_m15, R.id.cal_m16},
            {R.id.cal_m20, R.id.cal_m21, R.id.cal_m22, R.id.cal_m23, R.id.cal_m24, R.id.cal_m25, R.id.cal_m26},
            {R.id.cal_m30, R.id.cal_m31, R.id.cal_m32, R.id.cal_m33, R.id.cal_m34, R.id.cal_m35, R.id.cal_m36},
            {R.id.cal_m40, R.id.cal_m41, R.id.cal_m42, R.id.cal_m43, R.id.cal_m44, R.id.cal_m45, R.id.cal_m46},
            {R.id.cal_m50, R.id.cal_m51, R.id.cal_m52, R.id.cal_m53, R.id.cal_m54, R.id.cal_m55, R.id.cal_m56}
    };

}
