// Небо следует за временем суток: от него зависят палитра, солнце и то,
// насколько ярко идёт дождь матрицы. Фаза ставится атрибутом data-sky на
// <html> — компоненты берут цвета токенами и про время не знают.

export type Phase = "dawn" | "day" | "dusk" | "night";

export function phaseAt(d = new Date()): Phase {
  const h = d.getHours();
  if (h >= 5 && h < 8) return "dawn";
  if (h >= 8 && h < 18) return "day";
  if (h >= 18 && h < 22) return "dusk";
  return "night";
}

export const greeting: Record<Phase, string> = {
  dawn: "Доброе утро",
  day: "Добрый день",
  dusk: "Добрый вечер",
  night: "Доброй ночи",
};

/** Ставит фазу сразу и сверяет раз в пять минут. Возвращает отписку. */
export function followSky(onChange?: (p: Phase) => void) {
  const apply = () => {
    const p = phaseAt();
    if (document.documentElement.dataset.sky !== p) {
      document.documentElement.dataset.sky = p;
      onChange?.(p);
    }
  };
  apply();
  const t = setInterval(apply, 5 * 60 * 1000);
  return () => clearInterval(t);
}
