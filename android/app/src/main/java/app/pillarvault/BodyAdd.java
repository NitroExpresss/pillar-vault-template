package app.pillarvault;

import android.app.Activity;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.ViewGroup;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.List;

/**
 * Окно «+ замер» виджета «Тело»: по полю на каждую колонку таблицы замеров
 * (вес, талия…), в подсказке — прошлое значение. Пишет POST /metric за
 * сегодня; пустое поле — колонку не меняли. Поверх рабочего стола.
 */
public class BodyAdd extends Activity {

    private final Handler ui = new Handler(Looper.getMainLooper());
    private final List<EditText> fields = new ArrayList<>();
    private TextView status;
    private Button go;
    private boolean busy;
    private Sky.Pal sky;
    private float dp;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(R.layout.widget_body_add);
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_VISIBLE
                | WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        setFinishOnTouchOutside(true);
        status = findViewById(R.id.madd_status);
        go = findViewById(R.id.madd_go);
        sky = Sky.now();
        dp = getResources().getDisplayMetrics().density;
        findViewById(R.id.madd_root).setBackgroundResource(sky.bg);
        ((TextView) findViewById(R.id.madd_title)).setTextColor(sky.title);
        status.setTextColor(sky.soft);
        GradientDrawable btn = new GradientDrawable();
        btn.setColor(sky.accent);
        btn.setCornerRadius(14 * dp);
        go.setBackground(btn);
        go.setTextColor(sky.onAccent);
        go.setOnClickListener(v -> send());

        new Thread(() -> {
            VaultApi.Reply r = VaultApi.get("body.json");
            ui.post(() -> {
                if (isFinishing()) return;
                if (r.code == 401) {
                    status.setText("Войди в приложение — окно возьмёт ту же сессию");
                    return;
                }
                try {
                    JSONObject j = new JSONObject(r.body);
                    build(j.optJSONArray("columns"), j.optJSONArray("last"));
                } catch (Exception e) {
                    status.setText("Нет связи с хранилищем");
                }
            });
        }).start();
    }

    private void build(JSONArray cols, JSONArray last) {
        if (cols == null || cols.length() == 0) {
            status.setText("В карточке замеров нет таблицы");
            return;
        }
        LinearLayout box = findViewById(R.id.madd_fields);
        for (int i = 0; i < cols.length(); i++) {
            LinearLayout row = new LinearLayout(this);
            row.setOrientation(LinearLayout.HORIZONTAL);
            row.setGravity(Gravity.CENTER_VERTICAL);
            row.setPadding(0, Math.round(4 * dp), 0, Math.round(4 * dp));

            TextView name = new TextView(this);
            name.setText(cols.optString(i));
            name.setTextColor(sky.soft);
            name.setTextSize(TypedValue.COMPLEX_UNIT_SP, 14);
            row.addView(name, new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1));

            EditText f = new EditText(this);
            f.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
            f.setSingleLine(true);
            f.setGravity(Gravity.END | Gravity.CENTER_VERTICAL);
            f.setTypeface(android.graphics.Typeface.MONOSPACE);
            f.setTextColor(sky.ink);
            f.setHintTextColor(sky.faint);
            f.setTextSize(TypedValue.COMPLEX_UNIT_SP, 17);
            f.setBackground(Sky.field(sky, dp));
            f.setPadding(Math.round(12 * dp), 0, Math.round(12 * dp), 0);
            String was = last != null ? last.optString(i) : "";
            if (!was.isEmpty()) f.setHint("было " + was);
            boolean lastField = i == cols.length() - 1;
            f.setImeOptions(lastField ? EditorInfo.IME_ACTION_DONE : EditorInfo.IME_ACTION_NEXT);
            if (lastField) f.setOnEditorActionListener((v, a, e) -> {
                if (a == EditorInfo.IME_ACTION_DONE) {
                    send();
                    return true;
                }
                return false;
            });
            row.addView(f, new LinearLayout.LayoutParams(Math.round(130 * dp), Math.round(44 * dp)));
            box.addView(row);
            fields.add(f);
        }
        status.setText("Пустое поле — эту колонку не меняли");
        go.setEnabled(true);
        fields.get(0).requestFocus();
    }

    private void send() {
        if (busy || fields.isEmpty()) return;
        List<String> kv = new ArrayList<>();
        kv.add("do");
        kv.add("add");
        boolean any = false;
        for (int i = 0; i < fields.size(); i++) {
            String v = fields.get(i).getText().toString().trim();
            kv.add("v" + i);
            kv.add(v);
            any |= !v.isEmpty();
        }
        if (!any) {
            status.setText("Впиши хотя бы одно значение");
            return;
        }
        busy = true;
        go.setEnabled(false);
        new Thread(() -> {
            String err = null;
            try {
                VaultApi.post("metric", kv.toArray(new String[0]));
            } catch (Exception e) {
                err = e.getMessage() != null ? e.getMessage() : "нет связи с хранилищем";
            }
            final String fail = err;
            BodyWidget.updateAll(getApplicationContext());
            CalendarWidget.updateAll(getApplicationContext()); // метка замера в календаре
            ui.post(() -> {
                busy = false;
                go.setEnabled(true);
                if (fail != null) {
                    status.setText("Не записано: " + fail);
                    return;
                }
                Toast.makeText(this, "Замер записан", Toast.LENGTH_SHORT).show();
                finish();
            });
        }).start();
    }
}
