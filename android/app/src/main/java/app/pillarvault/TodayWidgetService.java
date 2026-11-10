package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/** Строки виджета «Сегодня». Сеть — в onDataSetChanged: он идёт не в главном потоке. */
public class TodayWidgetService extends RemoteViewsService {

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private final BurnRows burn; // вкладка «Что горит»
        private boolean burnTab;
        private List<VaultApi.Task> tasks = new ArrayList<>();
        private String notice; // вместо списка: «войди», «нет связи», «пусто»

        Factory(Context ctx, int widgetId) {
            this.ctx = ctx;
            this.widgetId = widgetId;
            this.burn = new BurnRows(ctx, widgetId, R.layout.widget_today);
        }

        @Override
        public void onCreate() {
        }

        @Override
        public void onDataSetChanged() {
            burnTab = TodayWidget.TAB_BURN.equals(TodayWidget.tab(ctx, widgetId));
            if (burnTab) {
                burn.onDataSetChanged();
                return;
            }
            VaultApi.Today t = VaultApi.today(ctx);
            String head;
            if (t.needLogin) {
                tasks = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (t.error != null) {
                // прежний список оставляем: лучше вчерашние дела, чем пустота
                notice = tasks.isEmpty() ? t.error : null;
                head = "нет связи";
            } else {
                tasks = t.tasks;
                notice = tasks.isEmpty() ? "На сегодня дел нет. Нажми — открыть дашборд" : null;
                // сколько осталось — видно, не листая список
                int late = 0;
                for (VaultApi.Task x : tasks) if (x.late) late++;
                head = tasks.isEmpty() ? t.date : t.date + " · " + tasks.size() + (late > 0 ? " · просрочено " + late : "");
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today);
            rv.setTextViewText(R.id.widget_date, head);
            AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
        }

        @Override
        public int getCount() {
            if (burnTab) return burn.getCount();
            return notice != null ? 1 : tasks.size();
        }

        @Override
        public RemoteViews getViewAt(int pos) {
            if (burnTab) return burn.getViewAt(pos);
            if (notice != null) {
                RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_notice);
                rv.setTextViewText(R.id.notice_text, notice);
                Sky.notice(rv, Sky.now());
                rv.setOnClickFillInIntent(R.id.notice_text, new Intent()
                        .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/"));
                return rv;
            }
            VaultApi.Task t = tasks.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_item);
            rv.setTextViewText(R.id.item_text, t.text);
            rv.setTextViewText(R.id.item_due, t.due);
            rv.setViewVisibility(R.id.item_due, t.due.isEmpty() ? android.view.View.GONE : android.view.View.VISIBLE);
            Sky.Pal p = Sky.now();
            rv.setTextColor(R.id.item_text, p.ink);
            rv.setInt(R.id.item_check, "setBackgroundResource", p.check);
            rv.setTextColor(R.id.item_due, t.late ? p.ember : p.warm); // срок прошёл — красным
            rv.setOnClickFillInIntent(R.id.item_check, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "done").putExtra(TodayWidget.EXTRA_TEXT, t.text));
            rv.setViewVisibility(R.id.item_later, android.view.View.VISIBLE);
            rv.setTextColor(R.id.item_later, p.accent);
            rv.setOnClickFillInIntent(R.id.item_later, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "later").putExtra(TodayWidget.EXTRA_TEXT, t.text));
            rv.setOnClickFillInIntent(R.id.item_text, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, t.url));
            return rv;
        }

        @Override
        public RemoteViews getLoadingView() {
            return null;
        }

        @Override
        public int getViewTypeCount() {
            return 3; // дело, сообщение, заголовок зоны «Что горит»
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
