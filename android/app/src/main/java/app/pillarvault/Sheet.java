package app.pillarvault;

import android.app.Activity;
import android.content.Intent;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.TypedValue;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

/**
 * Основа окон чтения поверх рабочего стола (DayView, CardView): разметка
 * widget_sheet, цвета по небу, строки тела и «Открыть в приложении».
 * Приложение не запускается, пока не попросишь.
 */
abstract class Sheet extends Activity {

    final Handler ui = new Handler(Looper.getMainLooper());
    Sky.Pal sky;
    float dp;
    LinearLayout body;
    TextView status;

    /** Адрес экрана в приложении для «Открыть в приложении». */
    abstract String appUrl();

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(R.layout.widget_sheet);
        setFinishOnTouchOutside(true);
        sky = Sky.now();
        dp = getResources().getDisplayMetrics().density;
        body = findViewById(R.id.sheet_body);
        status = findViewById(R.id.sheet_status);

        findViewById(R.id.sheet_root).setBackgroundResource(sky.bg);
        ((TextView) findViewById(R.id.sheet_title)).setTextColor(sky.title);
        ((TextView) findViewById(R.id.sheet_sub)).setTextColor(sky.soft);
        status.setTextColor(sky.warm);
        TextView open = findViewById(R.id.sheet_open);
        open.setTextColor(sky.accent);
        open.setOnClickListener(v -> {
            startActivity(new Intent(this, MainActivity.class)
                    .putExtra(TodayWidget.EXTRA_URL, appUrl())
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            finish();
        });
        EditText field = findViewById(R.id.sheet_text);
        field.setTextColor(sky.ink);
        field.setHintTextColor(sky.soft);
        field.setBackground(Sky.field(sky, dp));
        Button go = findViewById(R.id.sheet_go);
        GradientDrawable btn = new GradientDrawable();
        btn.setColor(sky.accent);
        btn.setCornerRadius(14 * dp);
        go.setBackground(btn);
        go.setTextColor(sky.onAccent);

        // тело не выше ~60% экрана: окно остаётся окном, а не вторым приложением
        ScrollView scroll = findViewById(R.id.sheet_scroll);
        int max = Math.round(getResources().getDisplayMetrics().heightPixels * 0.6f);
        scroll.addOnLayoutChangeListener((v, l, t, r, b, ol, ot, or, ob) -> {
            if (b - t > max) {
                scroll.getLayoutParams().height = max;
                scroll.requestLayout();
            }
        });
    }

    void title(String t, String sub) {
        ((TextView) findViewById(R.id.sheet_title)).setText(t);
        TextView s = findViewById(R.id.sheet_sub);
        s.setText(sub);
        s.setVisibility(sub == null || sub.isEmpty() ? View.GONE : View.VISIBLE);
    }

    void showStatus(String s) {
        status.setText(s);
        status.setVisibility(View.VISIBLE);
    }

    /** Подзаголовок раздела тела: «Дела», «Столпы»… */
    TextView head(String text) {
        TextView h = text(text, 13, sky.soft);
        h.setPadding(0, Math.round(10 * dp), 0, Math.round(2 * dp));
        h.setTypeface(android.graphics.Typeface.create("sans-serif-medium", android.graphics.Typeface.NORMAL));
        body.addView(h);
        return h;
    }

    /** Строка тела. */
    TextView line(CharSequence text, int color) {
        TextView t = text(text, 15, color);
        t.setPadding(0, Math.round(5 * dp), 0, Math.round(5 * dp));
        body.addView(t);
        return t;
    }

    TextView text(CharSequence s, int sp, int color) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextColor(color);
        t.setTextSize(TypedValue.COMPLEX_UNIT_SP, sp);
        return t;
    }
}
