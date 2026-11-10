package app.pillarvault;

import android.app.Activity;
import android.content.ContentResolver;
import android.os.Build;
import android.provider.Settings;
import android.view.Display;
import android.view.Window;
import android.view.WindowManager;

/**
 * Частота кадров окна — как у экрана в настройках системы. Без просьбы
 * окна прошивки (Xiaomi/HyperOS и др.) держат стороннее приложение на
 * 60 Гц, и WebView — прокрутка, анимации — рисует 60 кадров даже на 120-
 * герцовом экране.
 *
 * Берём системную настройку («Частота обновления» в настройках экрана):
 * AOSP — peak_refresh_rate, Xiaomi — user_refresh_rate. Выставил человек
 * 60 — уважаем 60 (батарея). Не прочиталась или «авто/максимум» — берём
 * лучший режим экрана. Разрешение не меняем: только частоту.
 */
final class Refresh {

    private Refresh() {
    }

    static void apply(Activity a) {
        try {
            // getDisplay() у активности — с Android 11; раньше — через WindowManager
            Display d = Build.VERSION.SDK_INT >= 30 ? a.getDisplay() : a.getWindowManager().getDefaultDisplay();
            if (d == null) return;
            Display.Mode cur = d.getMode();
            float cap = systemCap(a.getContentResolver());
            Display.Mode best = cur;
            for (Display.Mode m : d.getSupportedModes()) {
                if (m.getPhysicalWidth() != cur.getPhysicalWidth() || m.getPhysicalHeight() != cur.getPhysicalHeight()) {
                    continue; // другое разрешение — не наше дело
                }
                float r = m.getRefreshRate();
                if (cap > 0 && r > cap + 0.5f) continue;
                if (r > best.getRefreshRate() + 0.5f) best = m;
            }
            Window w = a.getWindow();
            WindowManager.LayoutParams lp = w.getAttributes();
            if (lp.preferredDisplayModeId != best.getModeId()) {
                lp.preferredDisplayModeId = best.getModeId();
                w.setAttributes(lp);
            }
        } catch (RuntimeException ignored) {
            // не вышло — остаёмся на том, что дала система
        }
    }

    /** Потолок частоты из настроек системы; 0 — не знаем или «без потолка». */
    private static float systemCap(ContentResolver cr) {
        float cap = read(cr, "peak_refresh_rate");
        if (cap <= 0) cap = read(cr, "user_refresh_rate"); // Xiaomi: 60 / 90 / 120 / 144
        if (Float.isInfinite(cap) || Float.isNaN(cap)) return 0;
        return cap;
    }

    private static float read(ContentResolver cr, String key) {
        try {
            String v = Settings.System.getString(cr, key);
            if (v == null) v = Settings.Secure.getString(cr, key);
            return v == null ? 0 : Float.parseFloat(v.trim());
        } catch (RuntimeException e) {
            return 0; // скрытая настройка закрыта для приложений — не беда
        }
    }
}
