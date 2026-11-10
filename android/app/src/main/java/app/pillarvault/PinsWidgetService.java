package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/** Строки виджета «Важное»: заголовок и summary карточки. */
public class PinsWidgetService extends RemoteViewsService {

    @Override
    public RemoteViewsFactory onGetViewFactory(Intent intent) {
        return new Factory(getApplicationContext(),
                intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID));
    }

    private static final class Factory implements RemoteViewsFactory {
        private final Context ctx;
        private final int widgetId;
        private List<VaultApi.Pin> items = new ArrayList<>();
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
            VaultApi.Pins p = VaultApi.pins();
            String head;
            if (p.needLogin) {
                items = new ArrayList<>();
                notice = "Войди в приложение — виджет возьмёт ту же сессию";
                head = "нужен вход";
            } else if (p.error != null) {
                notice = items.isEmpty() ? p.error : null;
                head = "нет связи";
            } else {
                items = p.items;
                notice = items.isEmpty() ? "Пусто. Закрепи карточку кнопкой на её странице" : null;
                head = items.isEmpty() ? "" : String.valueOf(items.size());
            }
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_pins);
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
                        .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, "/"));
                return rv;
            }
            VaultApi.Pin it = items.get(pos);
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_pin_item);
            rv.setTextViewText(R.id.pin_title, it.title);
            rv.setTextViewText(R.id.pin_text, it.summary);
            rv.setViewVisibility(R.id.pin_text, it.summary.isEmpty() ? View.GONE : View.VISIBLE);
            Sky.Pal p = Sky.now();
            rv.setTextColor(R.id.pin_title, p.ink);
            rv.setTextColor(R.id.pin_text, p.soft);
            // прочитать — окном поверх рабочего стола; правка — оттуда «Открыть в приложении»
            rv.setOnClickFillInIntent(R.id.pin_row, new Intent()
                    .putExtra(TodayWidget.EXTRA_DO, "card").putExtra(WidgetAction.EXTRA_SRC, it.id));
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
