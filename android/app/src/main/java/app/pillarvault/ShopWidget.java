package app.pillarvault;

import android.content.Context;
import android.content.Intent;

/**
 * Виджет «Покупки»: что купить, по разделам tasks/shopping. Квадрат —
 * куплено, «+» — новая покупка в выбранный раздел.
 */
public class ShopWidget extends ListWidget {

    static final String ACTION_REFRESH = "app.pillarvault.SHOP_REFRESH";

    @Override
    int layout() {
        return R.layout.widget_shop;
    }

    @Override
    Class<?> service() {
        return ShopWidgetService.class;
    }

    @Override
    String home() {
        return "/#shopping";
    }

    @Override
    Intent addIntent(Context ctx) {
        return new Intent(ctx, WidgetAdd.class).putExtra(WidgetAdd.EXTRA_MODE, WidgetAdd.MODE_SHOP);
    }

    @Override
    String refreshAction() {
        return ACTION_REFRESH;
    }

    @Override
    int requestBase() {
        return 50;
    }

    @Override
    String title(Context ctx) {
        return ctx.getString(R.string.widget_shop_title);
    }

    @Override
    Compact.Gist gist(Context ctx) {
        VaultApi.Shop s = VaultApi.shop();
        Compact.Gist t = Compact.trouble(s.needLogin, s.error);
        if (t != null) return t;
        if (s.items.isEmpty()) return new Compact.Gist("✓", "всё куплено");
        StringBuilder b = new StringBuilder();
        for (VaultApi.ShopItem it : s.items) {
            if (b.length() > 0) b.append(", ");
            b.append(it.text);
            if (b.length() > 60) break;
        }
        return new Compact.Gist(String.valueOf(s.items.size()), b.toString());
    }

    static void refreshAll(Context ctx) {
        refreshKind(ctx, ShopWidget.class);
    }
}
