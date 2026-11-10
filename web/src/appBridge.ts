// Мост Android-приложения: объект window.PillarApp есть только внутри него
// (MainActivity, addJavascriptInterface). В браузере всё ниже — пустые ответы.

export type WidgetKind = "today" | "calendar" | "body" | "vows" | "shop" | "pins" | "capture" | "goals" | "food" | "fast";

interface PillarApp {
  kinds?(): string;
  widgetCount?(kind: string): number;
  pinWidget?(kind: string): boolean;
  update?(): void;
  versionCode?(): number;
  versionName?(): string;
}

const app = (): PillarApp | undefined => (window as unknown as { PillarApp?: PillarApp }).PillarApp;

/** Можно ли поставить этот виджет на рабочий стол — и ещё не поставлен ли он. */
export function canPin(kind: WidgetKind): boolean {
  const a = app();
  if (!a?.widgetCount || !a.pinWidget) return false;
  let kinds = "today,calendar"; // приложения до 1.0.8 не знают kinds()
  try {
    kinds = a.kinds?.() ?? kinds;
  } catch {
    /* старое приложение */
  }
  if (!kinds.split(",").includes(kind)) return false;
  try {
    return a.widgetCount(kind) === 0;
  } catch {
    return false;
  }
}

export function pin(kind: WidgetKind): boolean {
  try {
    return !!app()?.pinWidget?.(kind);
  } catch {
    return false;
  }
}

/** Обновление умеют приложения новее кнопки «Обновить приложение». */
export function updater() {
  const a = app();
  if (!a || typeof a.update !== "function") return null;
  let code = 0;
  let name = "";
  try {
    code = a.versionCode?.() ?? 0;
    name = a.versionName?.() ?? "";
  } catch {
    /* старое приложение */
  }
  return { code, name, run: () => a.update!() };
}
