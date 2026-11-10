package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.Paint;
import android.graphics.Path;
import android.os.Bundle;
import android.util.DisplayMetrics;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/**
 * Строки «Целей». График — картинкой (линий у виджетов нет), тот же, что в
 * приложении: ось 0–100% пути к цели, пунктир идеального темпа от старта к
 * сроку, линия «сегодня», линии результатов цветами --goal-1…4.
 */
public class GoalsWidgetService extends RemoteViewsService {

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    static String pct(double v) {
        double r = Math.round(v * 10) / 10.0;
        return (r == Math.rint(r) ? String.valueOf((long) r) : String.valueOf(r)).replace('.', ',');
    }

    static String pace(VaultApi.Goal g) {
        switch (g.pace) {
            case "ahead":
                return "опережает на " + pct(g.gap) + " п.п.";
            case "behind":
                return "отстаёт на " + pct(-g.gap) + " п.п.";
            case "done":
                return "достигнута";
            case "nodeadline":
                return "без срока";
            case "nodata":
                return "нет показаний";
            default:
                return "по графику";
        }
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private List<VaultApi.Goal> goals = new ArrayList<>();
        private String today = "";
        private String notice;

        Factory(Context ctx, int widgetId) {
            this.ctx = ctx;
            this.widgetId = widgetId;
        }

        @Override
        public void onCreate() {
        }

        @Override
        public void onDataSetChanged() {
            VaultApi.Goals g = VaultApi.goals();
            String head;
            if (g.needLogin) {
                goals = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (g.error != null) {
                notice = goals.isEmpty() ? g.error : null;
                head = "нет связи";
            } else {
                goals = g.items;
                today = g.today;
                notice = goals.isEmpty() ? "Целей с результатами нет: deadline и таблица «Ключевые результаты» в карточке цели" : null;
                int behind = 0;
                for (VaultApi.Goal x : goals) if ("behind".equals(x.pace)) behind++;
                head = behind > 0 ? "отстаёт " + behind : "";
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_goals);
            rv.setTextViewText(R.id.widget_date, head);
            AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
        }

        @Override
        public int getCount() {
            return notice != null ? 1 : goals.size();
        }

        @Override
        public RemoteViews getViewAt(int pos) {
            Sky.Pal p = Sky.now();
            if (notice != null) {
                RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_notice);
                rv.setTextViewText(R.id.notice_text, notice);
                Sky.notice(rv, p);
                rv.setOnClickFillInIntent(R.id.notice_text, new Intent()
                        .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/"));
                return rv;
            }
            VaultApi.Goal g = goals.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_goal_item);
            rv.setTextViewText(R.id.goal_title, g.title);
            rv.setTextColor(R.id.goal_title, p.ink);
            rv.setTextViewText(R.id.goal_pace, pace(g));
            rv.setTextColor(R.id.goal_pace, "behind".equals(g.pace) ? p.ember
                    : "ahead".equals(g.pace) || "done".equals(g.pace) ? p.accent : p.soft);
            String left = g.deadline.isEmpty() ? "срок не задан"
                    : g.daysLeft >= 0 ? g.daysLeft + " дн. до " + g.deadline : "срок прошёл " + g.deadline;
            rv.setTextViewText(R.id.goal_meta, left + ", пройдено " + pct(g.pct) + "%"
                    + (g.deadline.isEmpty() ? "" : ", времени " + pct(g.timePct) + "%"));
            rv.setTextColor(R.id.goal_meta, p.soft);

            StringBuilder krs = new StringBuilder();
            for (VaultApi.GoalKR k : g.krs) {
                if (krs.length() > 0) krs.append("   ");
                krs.append(k.name).append(' ').append(pct(k.pct)).append('%');
            }
            if (g.tasksAll > 0) {
                if (krs.length() > 0) krs.append("   ");
                krs.append("Дела ").append(g.tasksDone).append('/').append(g.tasksAll);
            }
            rv.setTextViewText(R.id.goal_krs, krs);
            rv.setTextColor(R.id.goal_krs, p.soft);

            Bitmap chart = chart(g, p);
            if (chart != null) {
                rv.setImageViewBitmap(R.id.goal_chart, chart);
                rv.setViewVisibility(R.id.goal_chart, android.view.View.VISIBLE);
            } else {
                rv.setViewVisibility(R.id.goal_chart, android.view.View.GONE);
            }
            rv.setOnClickFillInIntent(R.id.goal_row, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, g.url));
            return rv;
        }

        /** Ширина картинки — по ширине виджета: подписи не мельчают и не тянутся. */
        private Bitmap chart(VaultApi.Goal g, Sky.Pal sky) {
            boolean any = false;
            for (VaultApi.GoalKR k : g.krs) any |= !k.pcts.isEmpty();
            if (!any) return null;
            DisplayMetrics dm = ctx.getResources().getDisplayMetrics();
            Bundle o = AppWidgetManager.getInstance(ctx).getAppWidgetOptions(widgetId);
            boolean land = ctx.getResources().getConfiguration().orientation == Configuration.ORIENTATION_LANDSCAPE;
            int wDp = o.getInt(land ? AppWidgetManager.OPTION_APPWIDGET_MAX_WIDTH : AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH, 320);
            float d = Math.min(dm.density, 2.5f); // картинка в виджете ограничена по памяти
            int w = Math.max(160, Math.round((wDp - 30) * d)), h = Math.round(96 * d);
            Bitmap bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888);
            Canvas c = new Canvas(bmp);

            long d0 = day(g.start), d1 = Math.max(g.deadline.isEmpty() ? day(today) : day(g.deadline), day(today));
            for (VaultApi.GoalKR k : g.krs) {
                for (String dt : k.dates) {
                    d0 = Math.min(d0, day(dt));
                    d1 = Math.max(d1, day(dt));
                }
            }
            final long from = d0;
            final float span = Math.max(1, d1 - d0);
            final float left = 28 * d, right = w - 6 * d, top = 6 * d, bottom = h - 14 * d;

            Paint grid = new Paint(Paint.ANTI_ALIAS_FLAG);
            grid.setColor(sky.grid);
            grid.setStrokeWidth(d);
            Paint axis = new Paint(Paint.ANTI_ALIAS_FLAG);
            axis.setColor(sky.soft);
            axis.setTextSize(9 * d);
            axis.setTypeface(android.graphics.Typeface.MONOSPACE);
            for (int v = 0; v <= 100; v += 50) {
                float yy = y(v, top, bottom);
                c.drawLine(left, yy, right, yy, grid);
                axis.setTextAlign(Paint.Align.RIGHT);
                c.drawText(v + "%", left - 4 * d, yy + 3 * d, axis);
            }
            axis.setTextAlign(Paint.Align.LEFT);
            c.drawText(shortDate(g.start), left, h - 2 * d, axis);
            if (!g.deadline.isEmpty()) {
                axis.setTextAlign(Paint.Align.RIGHT);
                c.drawText("срок " + shortDate(g.deadline), right, h - 2 * d, axis);
                // идеальный темп: равномерно от старта к сроку
                Paint ideal = new Paint(Paint.ANTI_ALIAS_FLAG);
                ideal.setColor(sky.soft);
                ideal.setAlpha(170);
                ideal.setStyle(Paint.Style.STROKE);
                ideal.setStrokeWidth(1.2f * d);
                ideal.setPathEffect(new DashPathEffect(new float[]{4 * d, 4 * d}, 0));
                Path p = new Path();
                p.moveTo(x(day(g.start), from, span, left, right), y(0, top, bottom));
                p.lineTo(x(day(g.deadline), from, span, left, right), y(100, top, bottom));
                c.drawPath(p, ideal);
            }
            long now = day(today);
            if (now > d0 && now < d1) {
                Paint t = new Paint(Paint.ANTI_ALIAS_FLAG);
                t.setColor(sky.soft);
                t.setAlpha(120);
                t.setStrokeWidth(d);
                float tx = x(now, from, span, left, right);
                c.drawLine(tx, top, tx, bottom, t);
            }
            int[] colors = Sky.goalColors(sky);
            int i = 0;
            for (VaultApi.GoalKR k : g.krs) {
                int col = colors[i++ % colors.length];
                if (k.pcts.isEmpty()) continue;
                Paint line = new Paint(Paint.ANTI_ALIAS_FLAG);
                line.setColor(col);
                line.setStyle(Paint.Style.STROKE);
                line.setStrokeWidth(2 * d);
                line.setStrokeJoin(Paint.Join.ROUND);
                line.setStrokeCap(Paint.Cap.ROUND);
                Path path = new Path();
                float lx = 0, ly = 0;
                for (int j = 0; j < k.pcts.size(); j++) {
                    lx = x(day(k.dates.get(j)), from, span, left, right);
                    ly = y(k.pcts.get(j), top, bottom);
                    if (j == 0) path.moveTo(lx, ly);
                    else path.lineTo(lx, ly);
                }
                c.drawPath(path, line);
                Paint dot = new Paint(Paint.ANTI_ALIAS_FLAG);
                dot.setColor(col);
                c.drawCircle(lx, ly, 3.5f * d, dot);
            }
            return bmp;
        }

        private static float x(long day, long from, float span, float left, float right) {
            return left + (right - left) * (day - from) / span;
        }

        private static float y(double pct, float top, float bottom) {
            double v = Math.max(-10, Math.min(110, pct));
            return (float) (top + (1 - v / 100) * (bottom - top));
        }

        private static long day(String iso) {
            try {
                return java.time.LocalDate.parse(iso.substring(0, 10)).toEpochDay();
            } catch (Exception e) {
                return java.time.LocalDate.now().toEpochDay();
            }
        }

        private static String shortDate(String iso) {
            return iso.length() >= 10 ? iso.substring(8, 10) + "." + iso.substring(5, 7) : iso;
        }

        @Override
        public RemoteViews getLoadingView() {
            return null;
        }

        @Override
        public int getViewTypeCount() {
            return 2;
        }

        @Override
        public long getItemId(int pos) {
            return pos;
        }

        @Override
        public boolean hasStableIds() {
            return false;
        }

        @Override
        public void onDestroy() {
        }
    }
}
