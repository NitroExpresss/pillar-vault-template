package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/** Строки виджета «Покупки». */
public class ShopWidgetService extends RemoteViewsService {

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private List<VaultApi.ShopItem> items = new ArrayList<>();
        private String src = "tasks/shopping";
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
            VaultApi.Shop s = VaultApi.shop();
            String head;
            if (s.needLogin) {
                items = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (s.error != null) {
                notice = items.isEmpty() ? s.error : null;
                head = "нет связи";
            } else {
                items = s.items;
                src = s.src;
                notice = items.isEmpty() ? "Всё куплено. «+» — добавить" : null;
                head = items.isEmpty() ? "" : String.valueOf(items.size());
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_shop);
            rv.setTextViewText(R.id.widget_date, head);
            AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
        }

        @Override
        public int getCount() {
            return notice != null ? 1 : items.size();
        }

        @Override
        public RemoteViews getViewAt(int pos) {
            if (notice != null) {
                RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_notice);
                rv.setTextViewText(R.id.notice_text, notice);
                Sky.notice(rv, Sky.now());
                rv.setOnClickFillInIntent(R.id.notice_text, new Intent()
                        .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/#shopping"));
                return rv;
            }
            VaultApi.ShopItem it = items.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_item);
            rv.setTextViewText(R.id.item_text, it.text);
            // раздел — мелко справа, только когда меняется: строки идут по разделам
            boolean first = pos == 0 || !items.get(pos - 1).group.equals(it.group);
            rv.setTextViewText(R.id.item_due, it.group);
            Sky.Pal p = Sky.now();
            rv.setTextColor(R.id.item_text, p.ink);
            rv.setInt(R.id.item_check, "setBackgroundResource", p.check);
            rv.setTextColor(R.id.item_due, p.soft);
            rv.setViewVisibility(R.id.item_due, first && !it.group.isEmpty() ? View.VISIBLE : View.GONE);
            rv.setContentDescription(R.id.item_check, "Куплено");
            rv.setOnClickFillInIntent(R.id.item_check, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "bought").putExtra(TodayWidget.EXTRA_TEXT, it.text)
                    .putExtra(WidgetAction.EXTRA_SRC, src));
            rv.setOnClickFillInIntent(R.id.item_text, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/#shopping"));
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
