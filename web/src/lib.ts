/** Согласование числа: 1 день, 2 дня, 5 дней. */
export function plural(n: number, one: string, few: string, many: string) {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

/** «21 октября, среда» (так дату отдаёт сервер) → день недели и число отдельно. */
export function splitDate(ru: string): { weekday: string; date: string } {
  const [date, weekday = ""] = ru.split(", ");
  return { weekday, date };
}
