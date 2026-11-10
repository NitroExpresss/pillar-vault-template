package app.pillarvault;

import android.content.Context;
import android.webkit.CookieManager;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLDecoder;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/**
 * Запросы виджета к хранилищу. Своего входа у виджета нет: кука сессии
 * берётся у WebView приложения — вошёл в приложении, и виджет видит дела.
 */
final class VaultApi {

    static final class Task {
        final String text, due, url;
        final boolean late; // срок прошёл

        Task(String text, String due, String url, boolean late) {
            this.text = text;
            this.due = due;
            this.url = url;
            this.late = late;
        }
    }

    static final class ShopItem {
        final String text, group;

        ShopItem(String text, String group) {
            this.text = text;
            this.group = group;
        }
    }

    static final class Shop {
        String src = "tasks/shopping"; // карточка: в неё уходит «куплено»
        final List<ShopItem> items = new ArrayList<>();
        final List<String> sections = new ArrayList<>(); // куда класть новую покупку
        boolean needLogin;
        String error;
    }

    static final class FoodItem {
        final String time, text;

        FoodItem(String time, String text) {
            this.time = time;
            this.text = text;
        }
    }

    static final class Food {
        final List<FoodItem> items = new ArrayList<>();
        boolean needLogin;
        String error;
    }

    static final class Pin {
        final String id, title, summary, url;

        Pin(String id, String title, String summary, String url) {
            this.id = id;
            this.title = title;
            this.summary = summary;
            this.url = url;
        }
    }

    static final class Due {
        final String text, src, url, when, zone; // zone: over, now, next

        Due(String text, String src, String url, String when, String zone) {
            this.text = text;
            this.src = src;
            this.url = url;
            this.when = when;
            this.zone = zone;
        }
    }

    static final class Burn {
        final List<Due> items = new ArrayList<>();
        boolean needLogin;
        String error;
    }

    static final class GoalKR {
        final String name;
        final double pct;
        final List<String> dates = new ArrayList<>();
        final List<Double> pcts = new ArrayList<>();

        GoalKR(String name, double pct) {
            this.name = name;
            this.pct = pct;
        }
    }

    static final class Goal {
        String id, title, url, start, deadline, pace;
        double pct, timePct, gap;
        int daysLeft, tasksDone, tasksAll;
        final List<GoalKR> krs = new ArrayList<>();
    }

    static final class Goals {
        String today = "";
        final List<Goal> items = new ArrayList<>();
        boolean needLogin;
        String error;
    }

    static final class Pins {
        final List<Pin> items = new ArrayList<>();
        boolean needLogin;
        String error;
    }

    static final class Today {
        String date = "";
        final List<Task> tasks = new ArrayList<>();
        final List<String> pick = new ArrayList<>(); // можно поставить на сегодня
        boolean needLogin;
        String error; // нет связи и т. п.
    }

    private VaultApi() {
    }

    /** Дела на сегодня. Не бросает: ошибка уходит в поле error. */
    static Today today(Context ctx) {
        Today out = new Today();
        HttpURLConnection c = null;
        try {
            c = open("today.json");
            int code = c.getResponseCode();
            keepCookies(c);
            if (code == 401) {
                out.needLogin = true;
                return out;
            }
            if (code != 200) {
                out.error = "Сервер ответил " + code;
                return out;
            }
            JSONObject j = new JSONObject(read(c.getInputStream()));
            out.date = j.optString("date");
            JSONArray arr = j.optJSONArray("tasks");
            for (int i = 0; arr != null && i < arr.length(); i++) {
                JSONObject t = arr.getJSONObject(i);
                out.tasks.add(new Task(t.optString("text"), t.optString("due"), t.optString("url"), t.optBoolean("late")));
            }
            JSONArray pick = j.optJSONArray("pick");
            for (int i = 0; pick != null && i < pick.length(); i++) {
                out.pick.add(pick.optString(i));
            }
        } catch (Exception e) {
            out.error = "Нет связи с хранилищем";
        } finally {
            if (c != null) c.disconnect();
        }
        return out;
    }

    /** «Что горит» — сроки по зонам, подряд. Не бросает: ошибка уходит в поле error. */
    static Burn burn() {
        Burn out = new Burn();
        Reply r = get("burn.json");
        if (r.code == 401) {
            out.needLogin = true;
            return out;
        }
        if (r.code != 200) {
            out.error = r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code;
            return out;
        }
        try {
            JSONArray zones = new JSONObject(r.body).optJSONArray("zones");
            for (int z = 0; zones != null && z < zones.length(); z++) {
                JSONObject zone = zones.getJSONObject(z);
                JSONArray arr = zone.optJSONArray("items");
                for (int i = 0; arr != null && i < arr.length(); i++) {
                    JSONObject t = arr.getJSONObject(i);
                    out.items.add(new Due(t.optString("text"), t.optString("src"), t.optString("url"),
                            t.optString("when"), zone.optString("key")));
                }
            }
        } catch (Exception e) {
            out.error = "Хранилище ответило непонятно";
        }
        return out;
    }

    /** Цели как OKR (goals.json). Не бросает: ошибка уходит в поле error. */
    static Goals goals() {
        Goals out = new Goals();
        Reply r = get("goals.json");
        if (r.code == 401) {
            out.needLogin = true;
            return out;
        }
        if (r.code != 200) {
            out.error = r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code;
            return out;
        }
        try {
            JSONObject j = new JSONObject(r.body);
            out.today = j.optString("today");
            JSONArray arr = j.optJSONArray("goals");
            for (int i = 0; arr != null && i < arr.length(); i++) {
                JSONObject o = arr.getJSONObject(i);
                Goal g = new Goal();
                g.id = o.optString("id");
                g.title = o.optString("title");
                g.url = o.optString("url");
                g.start = o.optString("start");
                g.deadline = o.optString("deadline");
                g.pace = o.optString("pace");
                g.pct = o.optDouble("pct", 0);
                g.timePct = o.optDouble("timePct", 0);
                g.gap = o.optDouble("gap", 0);
                g.daysLeft = o.optInt("daysLeft");
                JSONArray t = o.optJSONArray("tasks");
                if (t != null && t.length() == 2) {
                    g.tasksDone = t.optInt(0);
                    g.tasksAll = t.optInt(1);
                }
                JSONArray krs = o.optJSONArray("krs");
                for (int k = 0; krs != null && k < krs.length(); k++) {
                    JSONObject kr = krs.getJSONObject(k);
                    GoalKR x = new GoalKR(kr.optString("name"), kr.optDouble("pct", 0));
                    JSONArray pts = kr.optJSONArray("points");
                    for (int p = 0; pts != null && p < pts.length(); p++) {
                        x.dates.add(pts.getJSONObject(p).optString("date"));
                        x.pcts.add(pts.getJSONObject(p).optDouble("pct", 0));
                    }
                    g.krs.add(x);
                }
                out.items.add(g);
            }
        } catch (Exception e) {
            out.error = "Хранилище ответило непонятно";
        }
        return out;
    }

    /** Важные заметки — закреплённые карточки. Не бросает: ошибка уходит в поле error. */
    static Pins pins() {
        Pins out = new Pins();
        Reply r = get("pins.json");
        if (r.code == 401) {
            out.needLogin = true;
            return out;
        }
        if (r.code != 200) {
            out.error = r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code;
            return out;
        }
        try {
            JSONArray arr = new JSONObject(r.body).optJSONArray("items");
            for (int i = 0; arr != null && i < arr.length(); i++) {
                JSONObject t = arr.getJSONObject(i);
                out.items.add(new Pin(t.optString("id"), t.optString("title"), t.optString("summary"), t.optString("url")));
            }
        } catch (Exception e) {
            out.error = "Хранилище ответило непонятно";
        }
        return out;
    }

    /** Покупки по разделам. Не бросает: ошибка уходит в поле error. */
    static Shop shop() {
        Shop out = new Shop();
        Reply r = get("shop.json");
        if (r.code == 401) {
            out.needLogin = true;
            return out;
        }
        if (r.code != 200) {
            out.error = r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code;
            return out;
        }
        try {
            JSONObject j = new JSONObject(r.body);
            out.src = j.optString("src", out.src);
            JSONArray arr = j.optJSONArray("items");
            for (int i = 0; arr != null && i < arr.length(); i++) {
                JSONObject t = arr.getJSONObject(i);
                out.items.add(new ShopItem(t.optString("text"), t.optString("group")));
            }
            JSONArray sec = j.optJSONArray("sections");
            for (int i = 0; sec != null && i < sec.length(); i++) {
                out.sections.add(sec.optString(i));
            }
        } catch (Exception e) {
            out.error = "Хранилище ответило непонятно";
        }
        return out;
    }

    /** Что съедено сегодня (notes/food). Не бросает: ошибка уходит в поле error. */
    static Food food() {
        Food out = new Food();
        Reply r = get("food.json");
        if (r.code == 401) {
            out.needLogin = true;
            return out;
        }
        if (r.code != 200) {
            out.error = r.code < 0 ? "Нет связи с хранилищем" : "Сервер ответил " + r.code;
            return out;
        }
        try {
            JSONArray arr = new JSONObject(r.body).optJSONArray("items");
            for (int i = 0; arr != null && i < arr.length(); i++) {
                JSONObject t = arr.getJSONObject(i);
                out.items.add(new FoodItem(t.optString("time"), t.optString("text")));
            }
        } catch (Exception e) {
            out.error = "Хранилище ответило непонятно";
        }
        return out;
    }

    /** Ответ сервера: код и тело; code = -1 — нет связи. */
    static final class Reply {
        final int code;
        final String body;

        Reply(int code, String body) {
            this.code = code;
            this.body = body;
        }
    }

    /** GET по пути от корня хранилища («calendar.json?cal=2026-10»). Не бросает. */
    static Reply get(String path) {
        HttpURLConnection c = null;
        try {
            c = open(path);
            int code = c.getResponseCode();
            keepCookies(c);
            return new Reply(code, code == 200 ? read(c.getInputStream()) : "");
        } catch (Exception e) {
            return new Reply(-1, "");
        } finally {
            if (c != null) c.disconnect();
        }
    }

    /** Закрыть дело дня — тот же POST /today, что у квадрата на дашборде. */
    static void markDone(Context ctx, String text) throws IOException {
        today("done", text);
    }

    /**
     * POST /today — то же, что формы блока «Сегодня»: do = new (своё дело),
     * add (из всех дел), done (выполнено). Сервер и при ошибке отвечает
     * редиректом, только с «?m=причина»: её и бросаем — покажем человеку.
     */
    static void today(String what, String text) throws IOException {
        post("today", "do", what, "text", text);
    }

    /**
     * POST формы на путь хранилища: пары «имя, значение». Отказ сервера
     * («?m=причина» в редиректе) бросается исключением с этой причиной.
     */
    static void post(String path, String... kv) throws IOException {
        StringBuilder form = new StringBuilder();
        for (int i = 0; i + 1 < kv.length; i += 2) {
            if (form.length() > 0) form.append('&');
            form.append(URLEncoder.encode(kv[i], "UTF-8")).append('=').append(URLEncoder.encode(kv[i + 1], "UTF-8"));
        }
        HttpURLConnection c = open(path);
        try {
            c.setRequestMethod("POST");
            c.setDoOutput(true);
            c.setInstanceFollowRedirects(false); // ответ — редирект на дашборд, сама страница не нужна
            c.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
            byte[] body = form.toString().getBytes(StandardCharsets.UTF_8);
            try (OutputStream os = c.getOutputStream()) {
                os.write(body);
            }
            int code = c.getResponseCode();
            keepCookies(c);
            if (code != 303 && code != 200) {
                throw new IOException(code == 401 ? "нужен вход в приложении" : "сервер ответил " + code);
            }
            // успех с сообщением («записано») сервер помечает X-Pillar-Ok —
            // иначе m в адресе значит отказ
            if (c.getHeaderField("X-Pillar-Ok") != null) return;
            String why = refusal(c.getHeaderField("Location"));
            if (why != null) throw new IOException(why);
        } finally {
            c.disconnect();
        }
    }

    /**
     * Причина отказа из редиректа: параметр m в запросе («/?m=…#shopping»).
     * Именно параметр, а не любое «m=» в строке — иначе путь вроде
     * «/?cm=…» или якорь сошли бы за отказ. null — отказа нет.
     */
    static String refusal(String loc) throws IOException {
        if (loc == null) return null;
        int hash = loc.indexOf('#');
        if (hash >= 0) loc = loc.substring(0, hash);
        int q = loc.indexOf('?');
        if (q < 0) return null;
        for (String kv : loc.substring(q + 1).split("&")) {
            if (kv.startsWith("m=")) return URLDecoder.decode(kv.substring(2), "UTF-8");
        }
        return null;
    }

    /**
     * Сервер продлевает сессию, пока ею пользуются (новый Set-Cookie раз в
     * сутки). Виджеты ходят мимо WebView — без этого продлённый билет
     * оставался бы только у них в ответе, а через неделю истекал и в приложении.
     */
    static void keepCookies(HttpURLConnection c) {
        java.util.List<String> set = c.getHeaderFields().get("Set-Cookie");
        if (set == null || set.isEmpty()) return;
        CookieManager cm = CookieManager.getInstance();
        String base = c.getURL().getProtocol() + "://" + c.getURL().getHost() + "/";
        for (String s : set) cm.setCookie(base, s);
        cm.flush();
    }

    /**
     * Соединение с хранилищем по сессии WebView. Открыто и для AppUpdate:
     * APK обновления читает вызывающий, он же соединение и закрывает.
     */
    static HttpURLConnection open(String path) throws IOException {
        String base = BuildConfig.VAULT_URL.endsWith("/") ? BuildConfig.VAULT_URL : BuildConfig.VAULT_URL + "/";
        HttpURLConnection c = (HttpURLConnection) new URL(base + path).openConnection();
        c.setConnectTimeout(10000);
        c.setReadTimeout(15000);
        c.setUseCaches(false);
        // без сессии сервер ответит 401, а не редиректом на форму входа,
        // который у POST неотличим от успеха (server/pages.go, requirePage)
        c.setRequestProperty("X-Pillar-App", BuildConfig.VERSION_NAME);
        String cookie = CookieManager.getInstance().getCookie(base);
        if (cookie != null) {
            c.setRequestProperty("Cookie", cookie);
        }
        return c;
    }

    private static String read(InputStream in) throws IOException {
        try (InputStream is = in; ByteArrayOutputStream buf = new ByteArrayOutputStream()) {
            byte[] b = new byte[8192];
            for (int n; (n = is.read(b)) > 0; ) {
                buf.write(b, 0, n);
            }
            return buf.toString("UTF-8");
        }
    }
}
