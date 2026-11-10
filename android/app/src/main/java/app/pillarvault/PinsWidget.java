package app.pillarvault;

import android.content.Context;
import android.content.Intent;

/**
 * Виджет «Важное»: закреплённые карточки (pin: true). Строка открывает
 * карточку в приложении; закрепляют и открепляют на самой карточке.
 */
public class PinsWidget extends ListWidget {

    static final String ACTION_REFRESH = "app.pillarvault.PINS_REFRESH";

    @Override
    int layout() {
        return R.layout.widget_pins;
    }

    @Override
    Class<?> service() {
        return PinsWidgetService.class;
    }

    @Override
    String home() {
        return "/";
    }

    @Override
    Intent addIntent(Context ctx) {
        return null; // закрепить можно только на карточке — там видно, что именно
    }

    @Override
    String refreshAction() {
        return ACTION_REFRESH;
    }

    @Override
    int requestBase() {
        return 60;
    }

    @Override
    String title(Context ctx) {
        return ctx.getString(R.string.widget_pins_title);
    }

    @Override
    Compact.Gist gist(Context ctx) {
        VaultApi.Pins p = VaultApi.pins();
        Compact.Gist t = Compact.trouble(p.needLogin, p.error);
        if (t != null) return t;
        if (p.items.isEmpty()) return new Compact.Gist("0", "закрепи карточку на её странице");
        return new Compact.Gist(String.valueOf(p.items.size()), p.items.get(0).title);
    }

    static void refreshAll(Context ctx) {
        refreshKind(ctx, PinsWidget.class);
    }
}
