package app.pillarvault;

import android.content.Context;
import android.content.Intent;

/**
 * Виджет «Цели»: каждая цель — название и темп («отстаёт на 12 п.п.»),
 * срок и пройденный путь, график пути результатов к сроку (как в приложении)
 * и результаты строкой. Нажатие — цель в приложении. Данные — /goals.json.
 */
public class GoalsWidget extends ListWidget {

    static final String ACTION_REFRESH = "app.pillarvault.GOALS_REFRESH";

    @Override
    int layout() {
        return R.layout.widget_goals;
    }

    @Override
    Class<?> service() {
        return GoalsWidgetService.class;
    }

    @Override
    String home() {
        return "/";
    }

    @Override
    Intent addIntent(Context ctx) {
        return null; // цель заводится в карточке — там формат и смысл
    }

    @Override
    String refreshAction() {
        return ACTION_REFRESH;
    }

    @Override
    int requestBase() {
        return 80;
    }

    @Override
    String title(Context ctx) {
        return ctx.getString(R.string.widget_goals_title);
    }

    @Override
    Compact.Gist gist(Context ctx) {
        VaultApi.Goals g = VaultApi.goals();
        Compact.Gist t = Compact.trouble(g.needLogin, g.error);
        if (t != null) return t;
        if (g.items.isEmpty()) return new Compact.Gist("—", "целей с результатами нет");
        VaultApi.Goal first = g.items.get(0);
        return new Compact.Gist(GoalsWidgetService.pct(first.pct) + "%", first.title + ", " + GoalsWidgetService.pace(first));
    }

    static void refreshAll(Context ctx) {
        refreshKind(ctx, GoalsWidget.class);
    }
}
