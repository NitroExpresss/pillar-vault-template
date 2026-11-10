package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;

/** Строки виджета «Столпы». Сеть — в onDataSetChanged: он идёт не в главном потоке. */
public class VowsWidgetService extends RemoteViewsService {

    static final String HELD = "✓", BROKE = "✗";

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    private static final class Vow {
        final String name, rule, mark;
        final int streak, record;

        Vow(JSONObject j) {
            name = j.optString("name");
            rule = j.optString("rule");
            mark = j.optString("mark");
            streak = j.optInt("streak");
            record = j.optInt("record");
        }
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private List<Vow> vows = new ArrayList<>();
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
            VaultApi.Reply r = VaultApi.get("vows.json");
            String head;
            if (r.code == 401) {
                vows = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (r.code != 200) {
                notice = vows.isEmpty() ? "Нет связи с хранилищем" : null; // лучше вчерашнее, чем пустота
                head = "нет связи";
            } else {
                List<Vow> next = new ArrayList<>();
                int held = 0;
                String date = "";
                try {
                    JSONObject j = new JSONObject(r.body);
                    date = j.optString("date");
                    held = j.optInt("held");
                    JSONArray arr = j.optJSONArray("vows");
                    for (int i = 0; arr != null && i < arr.length(); i++) {
                        next.add(new Vow(arr.getJSONObject(i)));
                    }
                } catch (Exception ignored) {
                }
                vows = next;
                notice = vows.isEmpty() ? "Столпов нет — они живут в pillars/daily-vows" : null;
                head = vows.isEmpty() ? date : date + " · " + held + "/" + vows.size();
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_vows);
            rv.setTextViewText(R.id.widget_date, head);
            AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
        }

        @Override
        public int getCount() {
            return notice != null ? 1 : vows.size();
        }

        @Override
        public RemoteViews getViewAt(int pos) {
            if (notice != null) {
                RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_notice);
                rv.setTextViewText(R.id.notice_text, notice);
                Sky.notice(rv, Sky.now());
                rv.setOnClickFillInIntent(R.id.notice_text, new Intent()
                        .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/"));
                return rv;
            }
            Vow v = vows.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_vows_item);
            rv.setTextViewText(R.id.vow_name, v.name);
            rv.setTextViewText(R.id.vow_rule, v.rule);
            rv.setViewVisibility(R.id.vow_rule, v.rule.isEmpty() ? View.GONE : View.VISIBLE);
            // стрик: огонь, пока цепь жива; рекорд — мелко, если он больше текущего
            String streak = v.streak > 0 ? "🔥" + v.streak : "—";
            if (v.record > v.streak) streak += "  ⟨" + v.record + "⟩";
            rv.setTextViewText(R.id.vow_streak, streak);
            Sky.Pal p = Sky.now();
            rv.setTextColor(R.id.vow_name, p.ink);
            rv.setTextColor(R.id.vow_rule, p.soft);
            rv.setTextColor(R.id.vow_streak, BROKE.equals(v.mark) ? p.ember : p.warm);

            boolean held = HELD.equals(v.mark), broke = BROKE.equals(v.mark);
            rv.setInt(R.id.vow_held, "setBackgroundResource", held ? p.markHeld : p.mark);
            rv.setTextColor(R.id.vow_held, held ? p.onAccent : p.accent);
            rv.setInt(R.id.vow_broke, "setBackgroundResource", broke ? R.drawable.widget_mark_broke : p.mark);
            rv.setTextColor(R.id.vow_broke, broke ? 0xFFFFFFFF : p.ember);

            rv.setOnClickFillInIntent(R.id.vow_held, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "vow").putExtra(TodayWidget.EXTRA_TEXT, v.name)
                    .putExtra(WidgetAction.EXTRA_MARK, HELD));
            rv.setOnClickFillInIntent(R.id.vow_broke, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "vow").putExtra(TodayWidget.EXTRA_TEXT, v.name)
                    .putExtra(WidgetAction.EXTRA_MARK, BROKE));
            rv.setOnClickFillInIntent(R.id.vow_text, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/doc/pillars/daily-vows"));
            return rv;
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
