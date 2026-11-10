package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/** Строки виджета «Еда»: что съедено и когда. */
public class FoodWidgetService extends RemoteViewsService {

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private List<VaultApi.FoodItem> items = new ArrayList<>();
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
            VaultApi.Food f = VaultApi.food();
            String head;
            if (f.needLogin) {
                items = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (f.error != null) {
                notice = items.isEmpty() ? f.error : null;
                head = "нет связи";
            } else {
                items = f.items;
                notice = items.isEmpty() ? "Сегодня ещё ничего. «+» — записать" : null;
                head = items.isEmpty() ? "" : String.valueOf(items.size());
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_food);
            rv.setTextViewText(R.id.widget_date, head);
            AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
        }

        @Override
        public int getCount() {
            return notice != null ? 1 : items.size();
        }

        @Override
        public RemoteViews getViewAt(int pos) {
            Intent open = new Intent().putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/#w-food");
            if (notice != null) {
                RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_notice);
                rv.setTextViewText(R.id.notice_text, notice);
                Sky.notice(rv, Sky.now());
                rv.setOnClickFillInIntent(R.id.notice_text, open);
                return rv;
            }
            VaultApi.FoodItem it = items.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_item);
            Sky.Pal p = Sky.now();
            rv.setViewVisibility(R.id.item_check, View.GONE); // отмечать нечего — это журнал
            rv.setTextViewText(R.id.item_text, it.text);
            rv.setTextColor(R.id.item_text, p.ink);
            rv.setTextViewText(R.id.item_due, it.time);
            rv.setTextColor(R.id.item_due, p.soft);
            rv.setViewVisibility(R.id.item_due, it.time.isEmpty() ? View.GONE : View.VISIBLE);
            rv.setOnClickFillInIntent(R.id.item_text, open);
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
