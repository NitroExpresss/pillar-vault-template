package app.pillarvault;

import android.content.Context;
import android.content.Intent;

/**
 * Виджет «Еда»: что съедено сегодня (notes/food), «+» — записать приём пищи.
 */
public class FoodWidget extends ListWidget {

    static final String ACTION_REFRESH = "app.pillarvault.FOOD_REFRESH";

    @Override
    int layout() {
        return R.layout.widget_food;
    }

    @Override
    Class<?> service() {
        return FoodWidgetService.class;
    }

    @Override
    String home() {
        return "/#w-food";
    }

    @Override
    Intent addIntent(Context ctx) {
        return new Intent(ctx, WidgetAdd.class).putExtra(WidgetAdd.EXTRA_MODE, WidgetAdd.MODE_FOOD);
    }

    @Override
    String refreshAction() {
        return ACTION_REFRESH;
    }

    @Override
    int requestBase() {
        return 90;
    }

    @Override
    String title(Context ctx) {
        return ctx.getString(R.string.widget_food_title);
    }

    @Override
    Compact.Gist gist(Context ctx) {
        VaultApi.Food f = VaultApi.food();
        Compact.Gist t = Compact.trouble(f.needLogin, f.error);
        if (t != null) return t;
        if (f.items.isEmpty()) return new Compact.Gist("0", "сегодня ничего не записано");
        StringBuilder b = new StringBuilder();
        for (VaultApi.FoodItem it : f.items) {
            if (b.length() > 0) b.append(", ");
            b.append(it.text);
            if (b.length() > 60) break;
        }
        return new Compact.Gist(String.valueOf(f.items.size()), b.toString());
    }

    static void refreshAll(Context ctx) {
        refreshKind(ctx, FoodWidget.class);
    }
}
