package app.pillarvault;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.Paint;
import android.graphics.Path;
import android.os.Bundle;
import android.util.DisplayMetrics;
import android.view.View;
import android.widget.RemoteViews;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.Locale;

/**
 * Виджет «Тело»: текущий вес, путь к цели и график веса. График —
 * картинка, нарисованная здесь под размер виджета: стандартных элементов
 * для линий у виджетов нет. Данные — /body.json по сессии WebView;
 * последний ответ хранится, чтобы без связи график не пропадал.
 */
public class BodyWidget extends AppWidgetProvider {

    static final String ACTION_REFRESH = "app.pillarvault.BODY_REFRESH";
    private static final String PREFS = "body_widget";

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        refresh(ctx, ids, goAsync());
        Sky.schedule(ctx);
    }

    @Override
    public void onAppWidgetOptionsChanged(Context ctx, AppWidgetManager mgr, int id, Bundle opts) {
        refresh(ctx, new int[]{id}, goAsync()); // растянули — перерисовать под новый размер
    }

    @Override
    public void onReceive(Context ctx, Intent intent) {
        super.onReceive(ctx, intent);
        if (ACTION_REFRESH.equals(intent.getAction())) updateAll(ctx);
    }

    /** Перечитать все виджеты «Тело» — после правок в приложении. */
    static void updateAll(Context ctx) {
        AppWidgetManager mgr = AppWidgetManager.getInstance(ctx);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(ctx, BodyWidget.class));
        if (ids.length > 0) refresh(ctx, ids, null);
    }

    private static void refresh(Context ctx, int[] ids, PendingResult pending) {
        final Context app = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                SharedPreferences p = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
                VaultApi.Reply r = VaultApi.get("body.json");
                String status = null, body = r.body;
                if (r.code == 200) {
                    p.edit().putString("json", body).apply();
                } else {
                    status = r.code == 401 ? "Войди в приложение — виджет возьмёт ту же сессию"
                            : "Нет связи — показаны прошлые замеры";
                    body = p.getString("json", null);
                }
                AppWidgetManager mgr = AppWidgetManager.getInstance(app);
                for (int id : ids) {
                    mgr.updateAppWidget(id, build(app, mgr, id, body, status));
                }
            } finally {
                if (pending != null) pending.finish();
            }
        }).start();
    }

    private static RemoteViews build(Context ctx, AppWidgetManager mgr, int id, String body, String status) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_body);
        Sky.Pal sky = Sky.now();
        rv.setInt(R.id.body_root, "setBackgroundResource", sky.bg);
        rv.setTextColor(R.id.body_title, sky.title);
        rv.setTextColor(R.id.body_when, sky.soft);
        rv.setTextColor(R.id.body_add, sky.accent);
        rv.setTextColor(R.id.body_refresh, sky.accent);
        rv.setTextColor(R.id.body_now, sky.ink);
        rv.setTextColor(R.id.body_left, sky.soft);
        rv.setTextColor(R.id.body_status, sky.warm);

        Intent open = new Intent(ctx, MainActivity.class)
                .putExtra(TodayWidget.EXTRA_URL, "/#metrics")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        rv.setOnClickPendingIntent(R.id.body_root, PendingIntent.getActivity(ctx, 200000 + id, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        // «+» — окно замера поверх рабочего стола
        rv.setOnClickPendingIntent(R.id.body_add, PendingIntent.getActivity(ctx, 5,
                new Intent(ctx, BodyAdd.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        rv.setOnClickPendingIntent(R.id.body_refresh, PendingIntent.getBroadcast(ctx, 4,
                new Intent(ctx, BodyWidget.class).setAction(ACTION_REFRESH),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));

        rv.setViewVisibility(R.id.body_status, status == null ? View.GONE : View.VISIBLE);
        if (status != null) rv.setTextViewText(R.id.body_status, status);

        JSONObject j = null;
        try {
            if (body != null) j = new JSONObject(body);
        } catch (Exception ignored) {
        }
        JSONArray pts = j == null ? null : j.optJSONArray("points");
        if (pts == null || pts.length() == 0) {
            rv.setTextViewText(R.id.body_now, "—");
            rv.setTextViewText(R.id.body_left, j == null ? "" : "замеров пока нет");
            rv.setViewVisibility(R.id.body_progress, View.GONE);
            return rv;
        }

        String unit = j.optString("unit");
        String title = j.optString("title", "Вес");
        JSONObject last = pts.optJSONObject(pts.length() - 1);
        rv.setTextViewText(R.id.body_title, "Тело, " + title.toLowerCase(Locale.ROOT)
                + (j.optBoolean("demo") ? " (пример)" : ""));
        rv.setTextViewText(R.id.body_when, "замер " + shortDate(last.optString("date")));
        rv.setTextViewText(R.id.body_now, num(last.optDouble("value")) + (unit.isEmpty() ? "" : " " + unit));

        JSONObject g = j.optJSONObject("goal");
        if (g != null) {
            rv.setTextViewText(R.id.body_left, "до цели " + num(g.optDouble("left")) + " · цель " + num(g.optDouble("target")));
            double rate = g.optDouble("rate");
            rv.setTextViewText(R.id.body_rate, rate > 0 ? "−" + num(rate) + " " + unit + "/нед" : "темпа нет");
            rv.setTextColor(R.id.body_rate, rate > 0 ? sky.accent : sky.warm);
            rv.setViewVisibility(R.id.body_progress, View.VISIBLE);
            rv.setProgressBar(R.id.body_progress, 100, g.optInt("pct"), false);
        } else {
            rv.setTextViewText(R.id.body_left, "");
            rv.setTextViewText(R.id.body_rate, "");
            rv.setViewVisibility(R.id.body_progress, View.GONE);
        }

        rv.setImageViewBitmap(R.id.body_chart, chart(ctx, mgr, id, pts, g, status != null, sky));
        return rv;
    }

    /**
     * Линия веса под текущий размер виджета. Размер — из опций виджета
     * (в dp, для портрета и альбома — разные), минус шапка и цифры.
     */
    private static Bitmap chart(Context ctx, AppWidgetManager mgr, int id, JSONArray pts, JSONObject goal, boolean stale, Sky.Pal sky) {
        // цвета ряда — по небу: на светлом линия темнее, на тёмном светлее
        final int LINE = sky.line, GRID = sky.grid, AXIS = sky.soft, GOAL = sky.accent;
        DisplayMetrics dm = ctx.getResources().getDisplayMetrics();
        Bundle o = mgr.getAppWidgetOptions(id);
        boolean land = ctx.getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
        int wDp = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MAX_WIDTH : AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH, 320);
        int hDp = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT : AppWidgetManager.OPTION_APPWIDGET_MAX_HEIGHT, 180);
        // шапка 36 + цифры ~32 + полоса 12 + поля ~20
        float density = Math.min(dm.density, 2.5f); // картинка в виджете ограничена по памяти
        int w = Math.max(120, Math.round((wDp - 20) * density));
        int h = Math.max(60, Math.round((hDp - 110) * density));

        Bitmap bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888);
        Canvas c = new Canvas(bmp);
        float d = density;

        double min = Double.MAX_VALUE, max = -Double.MAX_VALUE;
        for (int i = 0; i < pts.length(); i++) {
            double v = pts.optJSONObject(i).optDouble("value");
            min = Math.min(min, v);
            max = Math.max(max, v);
        }
        double target = goal == null ? Double.NaN : goal.optDouble("target");
        boolean goalIn = !Double.isNaN(target) && target >= min - (max - min) * 0.5 && target <= max + (max - min) * 0.5;
        if (goalIn) {
            min = Math.min(min, target);
            max = Math.max(max, target);
        }
        double pad = (max - min) * 0.12;
        if (pad == 0) pad = 1;
        min -= pad;
        max += pad;

        float left = 30 * d, right = w - 46 * d, top = 6 * d, bottom = h - 14 * d;
        Paint grid = new Paint(Paint.ANTI_ALIAS_FLAG);
        grid.setColor(GRID);
        grid.setStrokeWidth(d);
        Paint axis = new Paint(Paint.ANTI_ALIAS_FLAG);
        axis.setColor(AXIS);
        axis.setTextSize(9 * d);
        axis.setTypeface(android.graphics.Typeface.MONOSPACE);

        // три линии сетки с подписями шкалы
        for (int k = 0; k <= 2; k++) {
            double v = min + (max - min) * k / 2;
            float y = y(v, min, max, top, bottom);
            c.drawLine(left, y, right, y, grid);
            axis.setTextAlign(Paint.Align.RIGHT);
            c.drawText(num(v), left - 4 * d, y + 3 * d, axis);
        }
        // первая и последняя дата
        axis.setTextAlign(Paint.Align.LEFT);
        c.drawText(shortDate(pts.optJSONObject(0).optString("date")), left, h - 2 * d, axis);
        axis.setTextAlign(Paint.Align.RIGHT);
        c.drawText(shortDate(pts.optJSONObject(pts.length() - 1).optString("date")), right, h - 2 * d, axis);

        // цель — пунктир, если попадает в разумный диапазон графика
        if (goalIn) {
            Paint gp = new Paint(Paint.ANTI_ALIAS_FLAG);
            gp.setColor(GOAL);
            gp.setAlpha(150);
            gp.setStrokeWidth(1.2f * d);
            gp.setStyle(Paint.Style.STROKE);
            gp.setPathEffect(new DashPathEffect(new float[]{5 * d, 4 * d}, 0));
            float gy = y(target, min, max, top, bottom);
            Path gl = new Path();
            gl.moveTo(left, gy);
            gl.lineTo(right, gy);
            c.drawPath(gl, gp);
            Paint gt = new Paint(axis);
            gt.setColor(GOAL);
            gt.setTextAlign(Paint.Align.LEFT);
            c.drawText("цель " + num(target), right + 4 * d, gy + 3 * d, gt);
        }

        // линия веса: по датам, а не по номерам — пропуски в замерах видны
        long t0 = day(pts.optJSONObject(0).optString("date"));
        long t1 = day(pts.optJSONObject(pts.length() - 1).optString("date"));
        long span = Math.max(1, t1 - t0);
        Paint line = new Paint(Paint.ANTI_ALIAS_FLAG);
        line.setColor(LINE);
        line.setStrokeWidth(2.2f * d);
        line.setStyle(Paint.Style.STROKE);
        line.setStrokeJoin(Paint.Join.ROUND);
        line.setStrokeCap(Paint.Cap.ROUND);
        if (stale) line.setAlpha(140);
        Path path = new Path();
        float lx = left, ly = top;
        for (int i = 0; i < pts.length(); i++) {
            JSONObject p = pts.optJSONObject(i);
            float x = pts.length() == 1 ? (left + right) / 2
                    : left + (right - left) * (day(p.optString("date")) - t0) / (float) span;
            float yy = y(p.optDouble("value"), min, max, top, bottom);
            if (i == 0) path.moveTo(x, yy);
            else path.lineTo(x, yy);
            lx = x;
            ly = yy;
        }
        c.drawPath(path, line);
        Paint dot = new Paint(Paint.ANTI_ALIAS_FLAG);
        dot.setColor(LINE);
        c.drawCircle(lx, ly, 3.5f * d, dot);
        Paint lbl = new Paint(axis);
        lbl.setColor(LINE);
        lbl.setTextAlign(Paint.Align.LEFT);
        lbl.setTextSize(10 * d);
        c.drawText(num(pts.optJSONObject(pts.length() - 1).optDouble("value")), lx + 6 * d, ly + 3.5f * d, lbl);
        return bmp;
    }

    private static float y(double v, double min, double max, float top, float bottom) {
        return (float) (top + (max - v) * (bottom - top) / (max - min));
    }

    /** Номер дня от эпохи для «ГГГГ-ММ-ДД»: ось X — по дням, а не по номерам замеров. */
    private static long day(String iso) {
        try {
            int y = Integer.parseInt(iso.substring(0, 4)), m = Integer.parseInt(iso.substring(5, 7)), dd = Integer.parseInt(iso.substring(8, 10));
            return java.time.LocalDate.of(y, m, dd).toEpochDay();
        } catch (Exception e) {
            return 0;
        }
    }

    private static String shortDate(String iso) {
        return iso.length() >= 10 ? iso.substring(8, 10) + "." + iso.substring(5, 7) : iso;
    }

    /** Одна цифра после запятой, без «.0». */
    private static String num(double v) {
        double r = Math.round(v * 10) / 10.0;
        return r == Math.rint(r) ? String.valueOf((long) r) : String.valueOf(r);
    }
}
