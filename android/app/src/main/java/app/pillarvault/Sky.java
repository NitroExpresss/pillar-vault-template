package app.pillarvault;

import android.app.AlarmManager;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.graphics.drawable.GradientDrawable;
import android.widget.RemoteViews;

import java.util.Calendar;

/**
 * Небо виджетов — то же, что в приложении («Рассвет над кодом»,
 * web/src/phase.ts): рассвет 5–8, день 8–18, закат 18–22, ночь. Фаза задаёт
 * подложку (градиент неба, пятно солнца и стеклянная вуаль поверх) и
 * оттенки чернил. Тема только тёмная (решение хозяина 2026-10-22): все
 * фазы — тёмное небо и светлые чернила.
 *
 * CSS-переменных у RemoteViews нет, поэтому палитра — здесь, а виджет при
 * сборке красит свои id сам. На границе фазы будильник (Tick) пересобирает
 * все виджеты; опоздает — догонит плановое обновление раз в полчаса.
 */
final class Sky {

    static final int DAWN = 0, DAY = 1, DUSK = 2, NIGHT = 3;

    /** Чернила и подложки одной фазы. */
    static final class Pal {
        final int bg;          // подложка виджета целиком
        final int title, ink, soft, faint, accent, ember, warm, blue, onAccent, line, grid;
        final int check, mark, markHeld, calDay, calToday;

        Pal(int bg, int title, int ink, int soft, int faint, int accent, int ember,
            int warm, int blue, int onAccent, int line, int grid) {
            this.bg = bg;
            this.title = title;
            this.ink = ink;
            this.soft = soft;
            this.faint = faint;
            this.accent = accent;
            this.ember = ember;
            this.warm = warm;
            this.blue = blue;
            this.onAccent = onAccent;
            this.line = line;
            this.grid = grid;
            check = R.drawable.widget_check;
            mark = R.drawable.widget_mark;
            markHeld = R.drawable.widget_mark_held;
            calDay = R.drawable.widget_cal_day;
            calToday = R.drawable.widget_cal_today;
        }
    }

    // цвета — из web/src/styles.css и viewer/gate.css, по фазам
    private static final Pal[] PALS = {
            new Pal(R.drawable.widget_bg_dawn, 0xFFA8EBC4, 0xFFF0EEF2, 0xFFC4BFD0, 0xFF6E6880,
                    0xFF4FD18E, 0xFFFF8A66, 0xFFF0B48E, 0xFF8CC8FF, 0xFF07140D, 0xFFF2C46B, 0x30FFFFFF),
            new Pal(R.drawable.widget_bg_day, 0xFF8EE4AF, 0xFFE6F2EA, 0xFFA9C2C4, 0xFF4A6468,
                    0xFF3EE08A, 0xFFFF7A57, 0xFFE0AE8E, 0xFF7FC8F0, 0xFF03110A, 0xFFE8BC5E, 0x2CA9C2C4),
            new Pal(R.drawable.widget_bg_dusk, 0xFF9FF0C2, 0xFFF1F4EF, 0xFFC9D0D8, 0xFF7D7F99,
                    0xFF4FD18E, 0xFFFF8A66, 0xFFF2B48C, 0xFF8CC8FF, 0xFF07140D, 0xFFF2C46B, 0x33FFFFFF),
            new Pal(R.drawable.widget_bg_night, 0xFF8EE4AF, 0xFFE6F2EA, 0xFFA7BDB3, 0xFF3D5A4C,
                    0xFF3EE08A, 0xFFFF7A57, 0xFFD9A48A, 0xFF6FD0E6, 0xFF03110A, 0xFFE0B65A, 0x26A7BDB3),
    };

    private Sky() {
    }

    static int phase(Calendar now) {
        int h = now.get(Calendar.HOUR_OF_DAY);
        if (h >= 5 && h < 8) return DAWN;
        if (h >= 8 && h < 18) return DAY;
        if (h >= 18 && h < 22) return DUSK;
        return NIGHT;
    }

    static Pal now() {
        return PALS[phase(Calendar.getInstance())];
    }

    /**
     * Шапка виджета-списка (Сегодня, Столпы, Покупки, Важное): подложка,
     * заголовок, дата, «+», ↻ и надпись пустого списка — по общим id.
     */
    static void header(RemoteViews rv, Pal p) {
        rv.setInt(R.id.widget_root, "setBackgroundResource", p.bg);
        rv.setTextColor(R.id.widget_title, p.title);
        rv.setTextColor(R.id.widget_date, p.soft);
        rv.setTextColor(R.id.widget_add, p.accent);
        rv.setTextColor(R.id.widget_refresh, p.accent);
        rv.setTextColor(R.id.widget_empty, p.soft);
    }

    /** Строка-сообщение списка (нужен вход, пусто, нет связи). */
    static void notice(RemoteViews rv, Pal p) {
        rv.setTextColor(R.id.notice_text, p.soft);
    }

    /** Ряды графика целей — те же цвета, что в приложении (--goal-1…4). */
    static int[] goalColors(Pal p) {
        return new int[]{0xFF3987E5, 0xFFD95926, 0xFF199E70, 0xFFC98500};
    }

    /** Поле ввода окон поверх рабочего стола: стекло светлее или темнее неба. */
    static GradientDrawable field(Pal p, float dp) {
        GradientDrawable g = new GradientDrawable();
        g.setColor(0x4D000000);
        g.setStroke(Math.max(1, Math.round(dp)), p.grid);
        g.setCornerRadius(14 * dp);
        return g;
    }

    /** Следующая смена неба: 5, 8, 18 или 22 часа. */
    static long nextChange(Calendar now) {
        Calendar c = (Calendar) now.clone();
        c.set(Calendar.MINUTE, 0);
        c.set(Calendar.SECOND, 5); // чуть после границы — фаза уже сменилась
        c.set(Calendar.MILLISECOND, 0);
        int h = now.get(Calendar.HOUR_OF_DAY);
        int next = h < 5 ? 5 : h < 8 ? 8 : h < 18 ? 18 : h < 22 ? 22 : 29; // 29 — завтра в 5
        if (next == 29) {
            c.add(Calendar.DAY_OF_YEAR, 1);
            next = 5;
        }
        c.set(Calendar.HOUR_OF_DAY, next);
        return c.getTimeInMillis();
    }

    /**
     * Завести будильник на следующую смену неба. Неточный: точным нужно
     * отдельное разрешение, а минута опоздания небу не важна. Повторный вызов
     * перезаписывает тот же будильник.
     */
    static void schedule(Context ctx) {
        Context app = ctx.getApplicationContext();
        AlarmManager am = app.getSystemService(AlarmManager.class);
        if (am == null) return;
        PendingIntent pi = PendingIntent.getBroadcast(app, 900, new Intent(app, Tick.class),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        am.set(AlarmManager.RTC, nextChange(Calendar.getInstance()), pi);
    }

    /** Небо сменилось — пересобрать все виджеты и завести следующий будильник. */
    public static class Tick extends BroadcastReceiver {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            Widgets.restyleAll(ctx);
            schedule(ctx);
        }
    }
}
