package app.pillarvault;

import android.content.Intent;
import android.os.Bundle;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.EditorInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

/**
 * Окно дня из виджета «Календарь»: дела со сроком на этот день (квадрат —
 * сделано), столпы, замеры, дневник — и поле «дело на этот день»
 * (POST /plan). Данные — GET /day.json.
 */
public class DayView extends Sheet {

    static final String EXTRA_DATE = "date";

    private String date;
    private boolean busy;

    @Override
    String appUrl() {
        return "/?cal=" + date.substring(0, 7) + "&day=" + date + "#calendar";
    }

    @Override
    protected void onCreate(Bundle state) {
        date = getIntent().getStringExtra(EXTRA_DATE);
        if (date == null || date.length() < 10) date = new java.text.SimpleDateFormat("yyyy-MM-dd", java.util.Locale.ROOT).format(new java.util.Date());
        super.onCreate(state);
        title(date, null);

        findViewById(R.id.sheet_input).setVisibility(View.VISIBLE);
        EditText field = findViewById(R.id.sheet_text);
        field.setHint(R.string.day_add_hint);
        Button go = findViewById(R.id.sheet_go);
        go.setOnClickListener(v -> plan(field));
        field.setOnEditorActionListener((v, action, ev) -> {
            boolean enter = ev != null && ev.getKeyCode() == KeyEvent.KEYCODE_ENTER && ev.getAction() == KeyEvent.ACTION_DOWN;
            if (action == EditorInfo.IME_ACTION_DONE || enter) {
                plan(field);
                return true;
            }
            return false;
        });
        load();
    }

    private void load() {
        new Thread(() -> {
            VaultApi.Reply r = VaultApi.get("day.json?date=" + date);
            ui.post(() -> {
                if (isFinishing()) return;
                if (r.code == 401) {
                    showStatus("Войди в приложение — окно возьмёт ту же сессию");
                    return;
                }
                try {
                    show(new JSONObject(r.body));
                } catch (Exception e) {
                    showStatus("Нет связи с хранилищем");
                }
            });
        }).start();
    }

    private void show(JSONObject d) {
        body.removeAllViews();
        title(d.optString("label", date), d.optBoolean("isToday") ? "сегодня" : null);
        boolean any = false;

        JSONArray tasks = d.optJSONArray("tasks");
        if (tasks != null && tasks.length() > 0) {
            any = true;
            head("Дела");
            for (int i = 0; i < tasks.length(); i++) task(tasks.optJSONObject(i));
        }
        JSONArray vows = d.optJSONArray("vows");
        if (vows != null && vows.length() > 0) {
            any = true;
            head("Столпы");
            for (int i = 0; i < vows.length(); i++) {
                JSONObject v = vows.optJSONObject(i);
                boolean held = "✓".equals(v.optString("value"));
                line((held ? "✓  " : "✗  ") + v.optString("name"), held ? sky.accent : sky.ember);
            }
        }
        JSONArray metrics = d.optJSONArray("metrics");
        if (metrics != null && metrics.length() > 0) {
            any = true;
            head("Замеры");
            for (int i = 0; i < metrics.length(); i++) {
                JSONObject m = metrics.optJSONObject(i);
                line(m.optString("name") + "  " + m.optString("value"), sky.ink);
            }
        }
        JSONArray diary = d.optJSONArray("diary");
        if (diary != null && diary.length() > 0) {
            any = true;
            head("Дневник");
            for (int i = 0; i < diary.length(); i++) {
                JSONObject e = diary.optJSONObject(i);
                TextView when = text(e.optString("when"), 12, sky.soft);
                when.setPadding(0, Math.round(4 * dp), 0, 0);
                body.addView(when);
                line(e.optString("text"), sky.ink);
            }
        }
        if (!any) line("В этот день ничего не записано.", sky.soft);
    }

    /** Строка дела: квадрат «сделано» и текст; закрытое — зачёркнуто. */
    private void task(JSONObject t) {
        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);
        row.setGravity(Gravity.CENTER_VERTICAL);
        TextView box = new TextView(this);
        boolean done = t.optBoolean("done");
        box.setBackgroundResource(sky.check);
        box.setGravity(Gravity.CENTER);
        box.setText(done ? "✓" : "");
        box.setTextColor(sky.accent);
        box.setContentDescription("Сделано: " + t.optString("text"));
        TextView text = text(t.optString("text"), 15, done ? sky.soft : sky.ink);
        if (done) text.setPaintFlags(text.getPaintFlags() | android.graphics.Paint.STRIKE_THRU_TEXT_FLAG);
        row.addView(box, new LinearLayout.LayoutParams(Math.round(36 * dp), Math.round(36 * dp)));
        row.addView(text, new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1));
        if (!done) box.setOnClickListener(v -> done(t.optString("src"), t.optString("text"), box, text));
        body.addView(row);
    }

    private void done(String src, String text, TextView box, TextView label) {
        box.setOnClickListener(null);
        box.setText("✓");
        label.setTextColor(sky.soft);
        label.setPaintFlags(label.getPaintFlags() | android.graphics.Paint.STRIKE_THRU_TEXT_FLAG);
        new Thread(() -> {
            String err = null;
            try {
                VaultApi.post("task", "do", "done", "src", src, "text", text);
            } catch (Exception e) {
                err = e.getMessage() != null ? e.getMessage() : "нет связи с хранилищем";
            }
            Widgets.refreshAll(getApplicationContext());
            final String fail = err;
            ui.post(() -> {
                if (fail != null) {
                    showStatus("Дело не закрыто: " + fail);
                    load();
                }
            });
        }).start();
    }

    /** Дело на этот день — в бэклог «По датам», как с календаря на главной. */
    private void plan(EditText field) {
        String text = field.getText().toString().trim().replaceAll("\\s+", " ");
        if (busy || text.isEmpty()) return;
        busy = true;
        new Thread(() -> {
            String err = null;
            try {
                VaultApi.post("plan", "date", date, "text", text);
            } catch (Exception e) {
                err = e.getMessage() != null ? e.getMessage() : "нет связи с хранилищем";
            }
            Widgets.refreshAll(getApplicationContext());
            final String fail = err;
            ui.post(() -> {
                busy = false;
                if (fail != null) {
                    showStatus("Не записано: " + fail);
                    return;
                }
                field.setText("");
                Toast.makeText(this, "На " + date + ": " + text, Toast.LENGTH_SHORT).show();
                load();
            });
        }).start();
    }

    /** Интент окна дня — для клеток календаря. */
    static Intent of(android.content.Context ctx, String date) {
        return new Intent(ctx, DayView.class).putExtra(EXTRA_DATE, date)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
    }
}
