package app.pillarvault;

import android.appwidget.AppWidgetManager;
import android.content.Context;
import android.content.Intent;
import android.view.View;
import android.widget.RemoteViews;
import android.widget.RemoteViewsService;

import java.util.ArrayList;
import java.util.List;

/**
 * Ряды вкладки «Что горит» виджета «Сегодня»: заголовок зоны (просрочено,
 * сегодня, дальше) перед её первым делом, затем дела — квадрат «сделано»,
 * срок, «→» на завтра. Данные — /burn.json.
 */
final class BurnRows implements RemoteViewsService.RemoteViewsFactory {

    /** Строка списка: заголовок зоны (due == null) или дело. */
    private static final class Row {
        final String head;
        final VaultApi.Due due;

        Row(String head, VaultApi.Due due) {
            this.head = head;
            this.due = due;
        }
    }

    private final Context ctx;
    private final int widgetId;
    private final int headLayout; // разметка виджета, в шапку которого пишется «просрочено N»
    private List<Row> rows = new ArrayList<>();
    private String notice;

    BurnRows(Context ctx, int widgetId, int headLayout) {
        this.ctx = ctx;
        this.widgetId = widgetId;
        this.headLayout = headLayout;
    }

    @Override
    public void onCreate() {
    }

    @Override
    public void onDataSetChanged() {
        VaultApi.Burn b = VaultApi.burn();
        String head;
        if (b.needLogin) {
            rows = new ArrayList<>();
            notice = "Войди в приложение — виджет возьмёт ту же сессию";
            head = "нужен вход";
        } else if (b.error != null) {
            notice = rows.isEmpty() ? b.error : null;
            head = "нет связи";
        } else {
            List<Row> next = new ArrayList<>();
            int over = 0;
            String zone = "";
            for (VaultApi.Due d : b.items) {
                if (!d.zone.equals(zone)) {
                    zone = d.zone;
                    next.add(new Row(zoneTitle(zone, count(b.items, zone)), null));
                }
                next.add(new Row(null, d));
                if ("over".equals(d.zone)) over++;
            }
            rows = next;
            notice = rows.isEmpty() ? "Сроков нет — ничего не горит" : null;
            head = over > 0 ? "просрочено " + over : "";
        }
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), headLayout);
        rv.setTextViewText(R.id.widget_date, head);
        AppWidgetManager.getInstance(ctx).partiallyUpdateAppWidget(widgetId, rv);
    }

    private static int count(List<VaultApi.Due> all, String zone) {
        int n = 0;
        for (VaultApi.Due d : all) if (d.zone.equals(zone)) n++;
        return n;
    }

    private static String zoneTitle(String zone, int n) {
        switch (zone) {
            case "over":
                return "Просрочено  " + n;
            case "now":
                return "Сегодня  " + n;
            default:
                return "Дальше  " + n;
        }
    }

    @Override
    public int getCount() {
        return notice != null ? 1 : rows.size();
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
        Row row = rows.get(pos);
        if (row.due == null) {
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_burn_head);
            rv.setTextViewText(R.id.burn_head, row.head);
            String zone = pos + 1 < rows.size() && rows.get(pos + 1).due != null ? rows.get(pos + 1).due.zone : "";
            rv.setTextColor(R.id.burn_head, "over".equals(zone) ? p.ember : "now".equals(zone) ? p.accent : p.soft);
            return rv;
        }
        VaultApi.Due d = row.due;
        RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.widget_today_item);
        rv.setTextViewText(R.id.item_text, d.text);
        rv.setTextColor(R.id.item_text, p.ink);
        rv.setInt(R.id.item_check, "setBackgroundResource", p.check);
        rv.setTextViewText(R.id.item_due, d.when);
        rv.setViewVisibility(R.id.item_due, d.when.isEmpty() ? View.GONE : View.VISIBLE);
        rv.setTextColor(R.id.item_due, "over".equals(d.zone) ? p.ember : p.warm);
        rv.setViewVisibility(R.id.item_later, View.VISIBLE);
        rv.setTextColor(R.id.item_later, p.accent);
        rv.setContentDescription(R.id.item_check, "Сделано");
        rv.setOnClickFillInIntent(R.id.item_check, new Intent()
                .putExtra(TodayWidget.EXTRA_DO, "taskdone").putExtra(TodayWidget.EXTRA_TEXT, d.text)
                .putExtra(WidgetAction.EXTRA_SRC, d.src));
        rv.setOnClickFillInIntent(R.id.item_later, new Intent()
                .putExtra(TodayWidget.EXTRA_DO, "move").putExtra(TodayWidget.EXTRA_TEXT, d.text)
                .putExtra(WidgetAction.EXTRA_SRC, d.src));
        rv.setOnClickFillInIntent(R.id.item_text, new Intent()
                .putExtra(TodayWidget.EXTRA_DO, "open").putExtra(TodayWidget.EXTRA_URL, d.url));
        return rv;
    }

    @Override
    public RemoteViews getLoadingView() {
        return null;
    }

    @Override
    public int getViewTypeCount() {
        return 3;
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
