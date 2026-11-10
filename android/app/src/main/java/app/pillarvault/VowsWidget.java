package app.pillarvault;

import android.content.Context;
import android.content.Intent;

/**
 * Виджет «Столпы»: правила дня со стриком. ✓ — держал, ✗ — сорвался;
 * повторное нажатие снимает отметку (так же, как на дашборде).
 */
public class VowsWidget extends ListWidget {

    static final String ACTION_REFRESH = "app.pillarvault.VOWS_REFRESH";

    @Override
    int layout() {
        return R.layout.widget_vows;
    }

    @Override
    Class<?> service() {
        return VowsWidgetService.class;
    }

    @Override
    String home() {
        return "/";
    }

    @Override
    Intent addIntent(Context ctx) {
        return null; // столпы правятся в карточке, а не с рабочего стола
    }

    @Override
    String refreshAction() {
        return ACTION_REFRESH;
    }

    @Override
    int requestBase() {
        return 40;
    }

    @Override
    String title(Context ctx) {
        return ctx.getString(R.string.widget_vows_title);
    }

    @Override
    Compact.Gist gist(Context ctx) {
        VaultApi.Reply r = VaultApi.get("vows.json");
        if (r.code == 401) return Compact.trouble(true, null);
        if (r.code != 200) return Compact.trouble(false, "нет связи с хранилищем");
        try {
            org.json.JSONObject j = new org.json.JSONObject(r.body);
            org.json.JSONArray vows = j.optJSONArray("vows");
            int total = vows == null ? 0 : vows.length(), held = j.optInt("held");
            String sub = total == 0 ? "столпов нет" : held == total ? "весь сад в цвету" : held == 0 ? "сад ждёт отметок" : "отмечено " + held + " из " + total;
            return new Compact.Gist(held + "/" + total, sub);
        } catch (Exception e) {
            return Compact.trouble(false, "хранилище ответило непонятно");
        }
    }

    static void refreshAll(Context ctx) {
        refreshKind(ctx, VowsWidget.class);
    }
}
