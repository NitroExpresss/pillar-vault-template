package app.pillarvault;

import android.app.Activity;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ListView;
import android.widget.TextView;
import android.widget.Toast;

import java.util.List;

/**
 * Окно ввода виджетов поверх рабочего стола — приложение не открывается.
 *
 * «Сегодня» (по умолчанию): поле для своего дела и список всех открытых дел —
 * нажал, и оно на сегодня. POST /today, как формы дашборда.
 * «Покупки» (shop): поле «что купить» и разделы карточки — Enter кладёт в
 * первый раздел, нажатие на раздел — в него. POST /shop.
 * «Дневник» (diary): запись в несколько строк. POST /diary.
 * «Мысль» (thought): строка в шлюз inbox/ — разберётся потом. POST /capture.
 *
 * Окно красится по небу (Sky), как виджеты.
 */
public class WidgetAdd extends Activity {

    static final String EXTRA_MODE = "mode";
    static final String MODE_SHOP = "shop";
    static final String MODE_DIARY = "diary";
    static final String MODE_THOUGHT = "thought";
    static final String MODE_FOOD = "food";

    private final Handler ui = new Handler(Looper.getMainLooper());
    private EditText field;
    private Button go;
    private TextView status;
    private boolean busy;
    private String mode;
    private Sky.Pal sky;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(R.layout.widget_add);
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_VISIBLE
                | WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        setFinishOnTouchOutside(true);
        mode = getIntent().getStringExtra(EXTRA_MODE);
        if (mode == null) mode = "";

        field = findViewById(R.id.add_text);
        go = findViewById(R.id.add_go);
        status = findViewById(R.id.add_status);
        TextView title = findViewById(R.id.add_title);
        switch (mode) {
            case MODE_SHOP:
                title.setText(R.string.shop_add_title);
                ((TextView) findViewById(R.id.add_pick_title)).setText(R.string.shop_add_pick);
                field.setHint(R.string.shop_add_hint);
                break;
            case MODE_DIARY:
                title.setText(R.string.diary_add_title);
                field.setHint(R.string.diary_add_hint);
                // запись — в несколько строк: Enter переносит строку, сохраняет кнопка
                field.setSingleLine(false);
                field.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
                        | InputType.TYPE_TEXT_FLAG_MULTI_LINE);
                field.setMinLines(3);
                field.setMaxLines(8);
                field.getLayoutParams().height = ViewGroup.LayoutParams.WRAP_CONTENT;
                go.setText("✓");
                break;
            case MODE_THOUGHT:
                title.setText(R.string.thought_add_title);
                field.setHint(R.string.thought_add_hint);
                break;
            case MODE_FOOD:
                title.setText(R.string.food_add_title);
                field.setHint(R.string.food_add_hint);
                break;
            default:
        }
        paint();

        go.setOnClickListener(v -> submit(""));
        field.setOnEditorActionListener((v, action, ev) -> {
            boolean enter = ev != null && ev.getKeyCode() == KeyEvent.KEYCODE_ENTER && ev.getAction() == KeyEvent.ACTION_DOWN;
            if (!MODE_DIARY.equals(mode) && (action == EditorInfo.IME_ACTION_DONE || enter)) {
                submit("");
                return true;
            }
            return false;
        });
        field.requestFocus();
        if (mode.isEmpty() || MODE_SHOP.equals(mode)) loadPick();
    }

    /** Цвета окна — по небу, как у виджетов. */
    private void paint() {
        sky = Sky.now();
        float dp = getResources().getDisplayMetrics().density;
        findViewById(R.id.add_root).setBackgroundResource(sky.bg);
        ((TextView) findViewById(R.id.add_title)).setTextColor(sky.title);
        ((TextView) findViewById(R.id.add_pick_title)).setTextColor(sky.soft);
        status.setTextColor(sky.warm);
        field.setTextColor(sky.ink);
        field.setHintTextColor(sky.soft);
        field.setBackground(Sky.field(sky, dp));
        GradientDrawable btn = new GradientDrawable();
        btn.setColor(sky.accent);
        btn.setCornerRadius(14 * dp);
        go.setBackground(btn);
        go.setTextColor(sky.onAccent);
        ((ListView) findViewById(R.id.add_list)).setDivider(new android.graphics.drawable.ColorDrawable(sky.grid));
        ((ListView) findViewById(R.id.add_list)).setDividerHeight(Math.max(1, Math.round(dp)));
    }

    /** «Сегодня»: своё дело. «Покупки»: покупка в раздел group («» — в первый). */
    private void submit(String group) {
        switch (mode) {
            case MODE_SHOP:
                send("shop", field.getText().toString(), group);
                break;
            case MODE_DIARY:
                send("diary", field.getText().toString(), null);
                break;
            case MODE_THOUGHT:
                send("capture", field.getText().toString(), null);
                break;
            case MODE_FOOD:
                send("food", field.getText().toString(), null);
                break;
            default:
                send("new", field.getText().toString(), null);
        }
    }

    /** Список под полем: открытые дела или разделы покупок. */
    private void loadPick() {
        final boolean shop = MODE_SHOP.equals(mode);
        new Thread(() -> {
            List<String> pick;
            boolean needLogin;
            if (shop) {
                VaultApi.Shop s = VaultApi.shop();
                pick = s.sections;
                needLogin = s.needLogin;
            } else {
                VaultApi.Today t = VaultApi.today(this);
                pick = t.pick;
                needLogin = t.needLogin;
            }
            ui.post(() -> {
                if (isFinishing()) return;
                if (needLogin) {
                    showStatus("Войди в приложение — окно возьмёт ту же сессию");
                    return;
                }
                // один раздел — выбирать не из чего, хватит Enter
                if (pick.isEmpty() || (shop && pick.size() < 2)) return;
                showPick(pick, shop);
            });
        }).start();
    }

    private void showPick(List<String> pick, boolean shop) {
        ListView list = findViewById(R.id.add_list);
        list.setAdapter(new ArrayAdapter<String>(this, R.layout.widget_add_row, pick) {
            @Override
            public View getView(int pos, View convert, ViewGroup parent) {
                TextView v = (TextView) super.getView(pos, convert, parent);
                v.setTextColor(sky.ink);
                return v;
            }
        });
        list.setOnItemClickListener((parent, view, pos, id) -> {
            if (shop) {
                if (field.getText().toString().trim().isEmpty()) {
                    showStatus("Сначала напиши, что купить");
                    return;
                }
                submit(pick.get(pos));
            } else {
                send("add", pick.get(pos), null);
            }
        });
        // не больше ~6 строк: окно не должно закрыть весь экран
        int rowPx = Math.round(45 * getResources().getDisplayMetrics().density);
        list.getLayoutParams().height = Math.min(pick.size(), 6) * rowPx;
        list.requestLayout();
        findViewById(R.id.add_pick_title).setVisibility(View.VISIBLE);
        list.setVisibility(View.VISIBLE);
    }

    private void send(String what, String raw, String group) {
        // в дневнике строки — часть записи; в остальном — одна строка
        String text = "diary".equals(what) ? raw.trim() : raw.trim().replaceAll("\\s+", " ");
        if (busy || text.isEmpty()) return;
        busy = true;
        go.setEnabled(false);
        new Thread(() -> {
            String err = null;
            try {
                switch (what) {
                    case "shop":
                        VaultApi.post("shop", "text", text, "group", group == null ? "" : group);
                        break;
                    case "diary":
                        VaultApi.post("diary", "text", text);
                        break;
                    case "capture":
                        VaultApi.post("capture", "text", text);
                        break;
                    case "food":
                        VaultApi.post("food", "text", text);
                        break;
                    default:
                        VaultApi.today(what, text);
                }
            } catch (Exception e) {
                err = e.getMessage() != null ? e.getMessage() : "нет связи с хранилищем";
            }
            final String fail = err;
            Widgets.refreshAll(getApplicationContext());
            ui.post(() -> {
                busy = false;
                go.setEnabled(true);
                if (fail != null) {
                    showStatus("Не записано: " + fail);
                    return;
                }
                Toast.makeText(this, done(what, text), Toast.LENGTH_SHORT).show();
                finish();
            });
        }).start();
    }

    private static String done(String what, String text) {
        switch (what) {
            case "shop":
                return "Купить: " + text;
            case "diary":
                return "Записано в дневник";
            case "capture":
                return "В inbox: " + text;
            case "food":
                return "Съедено: " + text;
            default:
                return "На сегодня: " + text;
        }
    }

    private void showStatus(String s) {
        status.setText(s);
        status.setVisibility(View.VISIBLE);
    }
}
