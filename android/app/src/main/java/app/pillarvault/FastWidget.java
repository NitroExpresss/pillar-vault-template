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
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.RectF;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.widget.RemoteViews;

import org.json.JSONArray;
import org.json.JSONObject;

import java.time.OffsetDateTime;
import java.util.Calendar;

/**
 * Виджет «Голодание»: сколько идёт голод (живой секундомер), кольцо к цели
 * и неделя окон — когда ел, когда голодал. Считает сервер по журналу еды
 * (/fast.json); «+» — записать еду, она и обрывает голод.
 */
public class FastWidget extends AppWidgetProvider {

    static final String ACTION_REFRESH = "app.pillarvault.FAST_REFRESH";
    private static final String PREFS = "fast_widget";
    private static final long HOUR = 3600_000L, DAY = 24 * HOUR;
    private static final String[] WD = {"вс", "пн", "вт", "ср", "чт", "пт", "сб"};

    @Override
    public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        refresh(ctx, ids, goAsync());
        Sky.schedule(ctx);
    }

    @Override
    public void onAppWidgetOptionsChanged(Context ctx, AppWidgetManager mgr, int id, Bundle opts) {
        refresh(ctx, new int[]{id}, goAsync()); // растянули — неделя под новый размер
    }

    @Override
    public void onReceive(Context ctx, Intent intent) {
        super.onReceive(ctx, intent);
        if (ACTION_REFRESH.equals(intent.getAction())) updateAll(ctx);
    }

    static void updateAll(Context ctx) {
        AppWidgetManager mgr = AppWidgetManager.getInstance(ctx);
        int[] ids = mgr.getAppWidgetIds(new ComponentName(ctx, FastWidget.class));
        if (ids.length > 0) refresh(ctx, ids, null);
    }

    private static void refresh(Context ctx, int[] ids, PendingResult pending) {
        final Context app = ctx.getApplicationContext();
        new Thread(() -> {
            try {
                SharedPreferences p = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
                VaultApi.Reply r = VaultApi.get("fast.json");
                String status = null, body = r.body;
                if (r.code == 200) {
                    p.edit().putString("json", body).apply();
                } else {
                    status = r.code == 401 ? "Войди в приложение — виджет возьмёт ту же сессию"
                            : "Нет связи — показано прошлое";
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

    private static long millis(String iso) {
        try {
            return OffsetDateTime.parse(iso).toInstant().toEpochMilli();
        } catch (Exception e) {
            return 0;
        }
    }

    private static RemoteViews build(Context ctx, AppWidgetManager mgr, int id, String body, String status) {
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_fast);
        Sky.Pal sky = Sky.now();
        rv.setInt(R.id.fast_root, "setBackgroundResource", sky.bg);
        rv.setTextColor(R.id.fast_title, sky.title);
        rv.setTextColor(R.id.fast_stage, sky.soft);
        rv.setTextColor(R.id.fast_add, sky.accent);
        rv.setTextColor(R.id.fast_refresh, sky.accent);
        rv.setTextColor(R.id.fast_timer, sky.ink);
        rv.setTextColor(R.id.fast_of, sky.soft);
        rv.setTextColor(R.id.fast_status, sky.warm);

        Intent open = new Intent(ctx, MainActivity.class)
                .putExtra(TodayWidget.EXTRA_URL, "/#w-fast")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        rv.setOnClickPendingIntent(R.id.fast_root, PendingIntent.getActivity(ctx, 300000 + id, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        rv.setOnClickPendingIntent(R.id.fast_add, PendingIntent.getActivity(ctx, 6,
                new Intent(ctx, WidgetAdd.class).putExtra(WidgetAdd.EXTRA_MODE, WidgetAdd.MODE_FOOD)
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        rv.setOnClickPendingIntent(R.id.fast_refresh, PendingIntent.getBroadcast(ctx, 7,
                new Intent(ctx, FastWidget.class).setAction(ACTION_REFRESH),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));

        rv.setViewVisibility(R.id.fast_status, status == null ? View.GONE : View.VISIBLE);
        if (status != null) rv.setTextViewText(R.id.fast_status, status);

        JSONObject j = null;
        try {
            if (body != null) j = new JSONObject(body);
        } catch (Exception ignored) {
        }
        float d = Math.min(ctx.getResources().getDisplayMetrics().density, 2.5f);
        long now = System.currentTimeMillis();
        long last = j == null ? 0 : millis(j.optString("last"));
        double goal = j == null ? 36 : j.optDouble("goal", 36);
        if (last == 0) {
            rv.setChronometer(R.id.fast_timer, SystemClock.elapsedRealtime(), "—", false);
            rv.setTextViewText(R.id.fast_of, "");
            rv.setTextViewText(R.id.fast_stage, j == null ? "" : "запиши еду — пойдёт отсчёт");
            rv.setImageViewBitmap(R.id.fast_ring, ring(0, d, sky));
            return rv;
        }
        double h = (now - last) / (double) HOUR;
        // секундомер тикает сам: база — «столько назад» по часам загрузки
        rv.setChronometer(R.id.fast_timer, SystemClock.elapsedRealtime() - (now - last), null, true);
        rv.setTextViewText(R.id.fast_of, "из " + num(goal) + " ч · " + Math.round(h / goal * 100) + "%");
        String stage = j.optString("stage");
        String next = j.optString("next");
        double nextIn = j.optDouble("next_in", 0) - (now - millis(j.optString("now"))) / (double) HOUR;
        String line = stage + (next.isEmpty() || nextIn <= 0 ? "" : " · до «" + next + "» " + dur(nextIn));
        // протокол с окном еды — в шапке важнее этапа: когда можно есть
        JSONObject plan = j.optJSONObject("plan");
        if (plan != null) {
            double in = plan.optDouble("in", 0) - (now - millis(j.optString("now"))) / (double) HOUR;
            if (in > 0) {
                line = plan.optBoolean("open") ? "окно еды до " + plan.optString("end") + " · ещё " + dur(in)
                        : "окно " + plan.optString("start") + "–" + plan.optString("end") + " через " + dur(in);
            }
        }
        String proto = j.optString("protocol");
        rv.setTextViewText(R.id.fast_title, proto.isEmpty() ? ctx.getString(R.string.widget_fast_title) : proto);
        rv.setTextViewText(R.id.fast_stage, line);
        rv.setImageViewBitmap(R.id.fast_ring, ring(h / goal, d, sky));
        rv.setImageViewBitmap(R.id.fast_week, week(ctx, mgr, id, j.optJSONArray("windows"), now, d, sky));
        return rv;
    }

    private static String num(double v) {
        return v == Math.rint(v) ? String.valueOf((long) v) : String.valueOf(Math.round(v * 10) / 10.0);
    }

    private static String dur(double h) {
        long m = Math.round(h * 60);
        return m < 60 ? m + " мин" : (m / 60) + " ч" + (m % 60 == 0 ? "" : " " + (m % 60) + " мин");
    }

    /** Кольцо к цели: дорожка и дуга; сверх цели — вторая дуга внутри. */
    private static Bitmap ring(double share, float d, Sky.Pal sky) {
        int s = Math.round(104 * d);
        Bitmap bmp = Bitmap.createBitmap(s, s, Bitmap.Config.ARGB_8888);
        Canvas c = new Canvas(bmp);
        float sw = 9 * d;
        Paint p = new Paint(Paint.ANTI_ALIAS_FLAG);
        p.setStyle(Paint.Style.STROKE);
        p.setStrokeWidth(sw);
        p.setStrokeCap(Paint.Cap.ROUND);
        RectF o = new RectF(sw / 2 + d, sw / 2 + d, s - sw / 2 - d, s - sw / 2 - d);
        p.setColor(sky.grid);
        c.drawArc(o, 0, 360, false, p);
        if (share > 0) {
            p.setColor(share >= 1 ? sky.title : sky.accent);
            c.drawArc(o, -90, (float) (360 * Math.min(share, 1)), false, p);
        }
        if (share > 1) {
            float in = sw + 3 * d;
            RectF i = new RectF(o.left + in, o.top + in, o.right - in, o.bottom - in);
            p.setStrokeWidth(4 * d);
            p.setColor(sky.accent);
            c.drawArc(i, -90, (float) (360 * Math.min(share - 1, 1)), false, p);
        }
        return bmp;
    }

    /** Неделя по суткам: строка — день, полоса — 24 часа; еда, голод, сейчас. */
    private static Bitmap week(Context ctx, AppWidgetManager mgr, int id, JSONArray wins, long now, float d, Sky.Pal sky) {
        Bundle o = mgr.getAppWidgetOptions(id);
        boolean land = ctx.getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
        int wDp = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MAX_WIDTH : AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH, 300);
        int hDp = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT : AppWidgetManager.OPTION_APPWIDGET_MAX_HEIGHT, 150);
        // минус кольцо 104, поля и шапка 36
        int w = Math.max(80, Math.round((wDp - 104 - 30) * d));
        int h = Math.max(60, Math.round((hDp - 56) * d));
        Bitmap bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888);
        Canvas c = new Canvas(bmp);

        Calendar cal = Calendar.getInstance();
        cal.setTimeInMillis(now);
        cal.set(Calendar.HOUR_OF_DAY, 0);
        cal.set(Calendar.MINUTE, 0);
        cal.set(Calendar.SECOND, 0);
        cal.set(Calendar.MILLISECOND, 0);
        long today = cal.getTimeInMillis();

        int rows = 7;
        float label = 30 * d;
        float axis = 12 * d;
        float rowH = (h - axis) / rows;
        float bar = Math.min(rowH * 0.62f, 12 * d);
        float x0 = label, x1 = w - 2 * d;
        Paint text = new Paint(Paint.ANTI_ALIAS_FLAG);
        text.setColor(sky.soft);
        text.setTextSize(Math.min(10 * d, rowH * 0.8f));
        Paint fill = new Paint(Paint.ANTI_ALIAS_FLAG);
        int fastCol = Color.argb(115, Color.red(sky.accent), Color.green(sky.accent), Color.blue(sky.accent));

        for (int r = 0; r < rows; r++) {
            long d0 = today - r * DAY; // переход на зимнее время сдвинет полосу на час — терпимо
            long d1 = d0 + DAY;
            float cy = r * rowH + rowH / 2;
            float top = cy - bar / 2, bot = cy + bar / 2;
            cal.setTimeInMillis(d0);
            c.drawText(WD[cal.get(Calendar.DAY_OF_WEEK) - 1] + " " + cal.get(Calendar.DAY_OF_MONTH), 0, cy + text.getTextSize() / 3, text);
            fill.setColor(sky.grid);
            c.drawRoundRect(new RectF(x0, top, x1, bot), bar / 2, bar / 2, fill);
            for (int i = 0; wins != null && i < wins.length(); i++) {
                JSONObject win = wins.optJSONObject(i);
                String kind = win.optString("kind");
                long a = millis(win.optString("start"));
                long b = "now".equals(kind) ? now : millis(win.optString("end"));
                if (b <= d0 || a >= d1) continue;
                float xa = x0 + (x1 - x0) * (Math.max(a, d0) - d0) / (float) DAY;
                float xb = x0 + (x1 - x0) * (Math.min(b, d1) - d0) / (float) DAY;
                if ("eat".equals(kind)) {
                    fill.setColor(sky.ember);
                    xb = Math.max(xb, xa + 3 * d);
                } else {
                    fill.setColor("now".equals(kind) ? sky.accent : fastCol);
                }
                c.drawRect(xa, top, xb, bot, fill);
            }
        }
        // ось часов
        text.setTextSize(9 * d);
        for (int t = 0; t <= 24; t += 6) {
            String s = String.valueOf(t);
            float x = x0 + (x1 - x0) * t / 24f - (t == 0 ? 0 : t == 24 ? text.measureText(s) : text.measureText(s) / 2);
            c.drawText(s, x, h - 2 * d, text);
        }
        return bmp;
    }
}
