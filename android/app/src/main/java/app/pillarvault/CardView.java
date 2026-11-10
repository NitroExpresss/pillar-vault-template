package app.pillarvault;

import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.text.Html;
import android.text.SpannableStringBuilder;
import android.text.method.LinkMovementMethod;
import android.text.style.ClickableSpan;
import android.text.style.URLSpan;
import android.view.View;
import android.widget.TextView;

import org.json.JSONObject;

/**
 * Карточка из «Важного» — почитать, не открывая приложение. Разметку отдаёт
 * сервер (GET /api/doc, сырой HTML там уже экранирован); здесь её рисует
 * Html.fromHtml: заголовки, списки, жирный, ссылки. Правка, связи и полная
 * вёрстка — «Открыть в приложении».
 */
public class CardView extends Sheet {

    static final String EXTRA_ID = "id";

    private String id;

    @Override
    String appUrl() {
        return "/doc/" + id;
    }

    @Override
    protected void onCreate(Bundle state) {
        id = getIntent().getStringExtra(EXTRA_ID);
        if (id == null) id = "";
        super.onCreate(state);
        title(id, null);
        new Thread(() -> {
            VaultApi.Reply r;
            try {
                r = VaultApi.get("api/doc?id=" + java.net.URLEncoder.encode(id, "UTF-8"));
            } catch (Exception e) {
                r = new VaultApi.Reply(-1, "");
            }
            final VaultApi.Reply reply = r;
            ui.post(() -> {
                if (isFinishing()) return;
                if (reply.code == 401) {
                    showStatus("Войди в приложение — окно возьмёт ту же сессию");
                    return;
                }
                if (reply.code == 404) {
                    showStatus("Такой карточки больше нет");
                    return;
                }
                try {
                    JSONObject d = new JSONObject(reply.body);
                    title(d.optString("title", id), d.optString("summary"));
                    TextView t = line(links(Html.fromHtml(d.optString("html"), Html.FROM_HTML_MODE_COMPACT)), sky.ink);
                    t.setLinkTextColor(sky.accent);
                    t.setLineSpacing(0, 1.15f);
                    t.setMovementMethod(LinkMovementMethod.getInstance());
                } catch (Exception e) {
                    showStatus("Нет связи с хранилищем");
                }
            });
        }).start();
    }

    /**
     * Ссылки разметки — относительные («/doc/…»): системный URLSpan такую
     * открыть не может и уронил бы окно. Вики-ссылка открывает карточку тут
     * же, другой свой адрес — в приложении, внешний — в браузере.
     */
    private CharSequence links(CharSequence html) {
        SpannableStringBuilder s = new SpannableStringBuilder(html);
        for (URLSpan u : s.getSpans(0, s.length(), URLSpan.class)) {
            final String href = u.getURL();
            int a = s.getSpanStart(u), b = s.getSpanEnd(u), f = s.getSpanFlags(u);
            s.removeSpan(u);
            s.setSpan(new ClickableSpan() {
                @Override
                public void onClick(View w) {
                    follow(href);
                }
            }, a, b, f);
        }
        return s;
    }

    private void follow(String href) {
        try {
            if (href.startsWith("/doc/")) {
                startActivity(of(this, Uri.decode(href.substring(5))));
            } else if (href.startsWith("/")) {
                startActivity(new Intent(this, MainActivity.class).putExtra(TodayWidget.EXTRA_URL, href)
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            } else {
                startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(href)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            }
            finish();
        } catch (Exception e) {
            showStatus("Ссылку нечем открыть: " + href);
        }
    }

    /** Интент окна карточки — для строк «Важного». */
    static Intent of(android.content.Context ctx, String id) {
        return new Intent(ctx, CardView.class).putExtra(EXTRA_ID, id)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
    }
}
