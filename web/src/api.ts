// Разговор с Go-сервером. Чтения — JSON по экрану, записи — те же формы,
// что у старого интерфейса, с заголовком X-Pillar-Api: сервер отвечает
// {"ok","message"} вместо редиректа (server/api.go).

const API = { "X-Pillar-Api": "1" };

/** Сессия кончилась — на форму входа: она живёт на Go, не в приложении,
 *  и после ключа вернёт на этот же экран. */
function toGate(): never {
  const here = location.pathname + location.search;
  window.location.assign(here === "/" ? "/" : "/?next=" + encodeURIComponent(here));
  throw new Error("нужен вход");
}

export async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const r = await fetch(url, { headers: API, credentials: "same-origin", cache: "no-store", signal });
  if (r.status === 401) toGate();
  if (!r.ok) throw new Error(`${url}: ${r.status}`);
  return (await r.json()) as T;
}

export interface Result {
  ok: boolean;
  message?: string;
}

/** Форма, ответ — JSON целиком (не только ok/message): чат с ИИ, разбор. */
export async function postJSON<T extends { ok: boolean }>(path: string, fields: Record<string, string>): Promise<T> {
  let r: Response;
  try {
    r = await fetch(path, { method: "POST", headers: API, credentials: "same-origin", body: new URLSearchParams(fields) });
  } catch {
    return { ok: false, message: "нет связи с сервером" } as unknown as T;
  }
  if (r.status === 401) toGate();
  if ((r.headers.get("Content-Type") || "").includes("application/json")) return (await r.json()) as T;
  return { ok: false, message: (await r.text()).trim() || `ошибка ${r.status}` } as unknown as T;
}

/** Форма на обработчик сервера. Отказ — ok:false с причиной, а не исключение. */
export async function send(path: string, fields: Record<string, string>): Promise<Result> {
  let r: Response;
  try {
    r = await fetch(path, { method: "POST", headers: API, credentials: "same-origin", body: new URLSearchParams(fields) });
  } catch {
    return { ok: false, message: "нет связи с сервером" };
  }
  if (r.status === 401) toGate();
  if ((r.headers.get("Content-Type") || "").includes("application/json")) return (await r.json()) as Result;
  return { ok: false, message: (await r.text()).trim() || `ошибка ${r.status}` };
}

// ── формы ответов (server/widget.go, server/api.go) ──

export interface TodayTask {
  text: string;
  due?: string;
  late?: boolean;
  url: string;
}

export interface Today {
  date: string;
  tasks: TodayTask[];
  pick: string[];
}

export interface Vow {
  name: string;
  rule: string;
  streak: number;
  record: number;
  mark: "" | "✓" | "✗";
}

export interface Vows {
  date: string;
  held: number;
  vows: Vow[];
}

export interface NavDoc {
  id: string;
  title: string;
  url: string;
}

export interface NavDir {
  path: string;
  label: string;
  count: number;
  url: string;
  dirs: NavDir[];
  files: NavDoc[];
}

export interface NavTag {
  name: string;
  count: number;
  share: number;
  url: string;
}

export interface Nav {
  dirs: NavDir[];
  files: NavDoc[];
  tags: NavTag[];
  cards: number;
  inbox: number; // мыслей ждёт разбора
  broken: number; // ошибок линтера
}

// ── дашборд одним ответом (server/board.go) ──

export interface BoardTask {
  text: string;
  src: string;
  url: string;
  due?: string;
  done?: boolean;
}

export interface CalDay {
  date: string;
  day: number;
  in: boolean;
  today?: boolean;
  due?: number;
  diary?: boolean;
  metric?: boolean;
  vowHeld?: number;
  vowBroke?: number;
}

export interface Pair {
  name: string;
  value: string;
}

export interface Entry {
  when: string;
  text: string;
}

export interface Board {
  today: string;
  calendar: {
    title: string;
    month: string;
    prev: string;
    next: string;
    week: string[];
    weeks: CalDay[][];
    day: { date: string; label: string; isToday: boolean; tasks: BoardTask[]; vows: Pair[]; metrics: Pair[]; diary: Entry[] };
  };
  body?: {
    title: string;
    url: string;
    demo?: boolean;
    charts: string[];
    goal?: { now: number; target: number; start: number; left: number; pct: number; rate: number };
    fields: { idx: number; name: string; last?: string }[];
  };
  diary: Entry[];
  deadlines: { key: "over" | "now" | "next"; title: string; items: (BoardTask & { when: string })[] }[];
  wheel: {
    name: string; score: number; max: number; pct: number; low?: boolean; goals?: string[];
    cards?: { title: string; url: string }[]; // цели, планы, проекты с sphere: этой сферы
  }[];
  tasks?: { open: number; done: number; pct: number; groups: { name: string; items: BoardTask[] }[] };
  gallery?: { title?: string; note?: string; autoplayMs?: number; mocks: number; slides: { src: string; caption: string; sub: string }[] };
  pins: Pinned[]; // важные заметки — карточки с pin: true
  goals: Goal[]; // цели как OKR (server/goals.go)
}

export interface GoalKR {
  name: string;
  unit?: string;
  start: number;
  current: number;
  target: number;
  pct: number;
  points: { date: string; value: number; pct: number }[];
  from?: string; // показания из журнала замеров
  col: number; // колонка таблицы: v<col> в POST /goal
}

export interface Goal {
  id: string;
  title: string;
  url: string;
  start: string;
  deadline?: string;
  daysLeft: number;
  timePct: number;
  pct: number;
  pace: "ahead" | "on" | "behind" | "done" | "nodeadline" | "nodata";
  gap: number;
  krs: GoalKR[];
  tasks: [number, number];
}

export interface Pinned {
  id: string;
  title: string;
  summary?: string;
  url: string;
}

/** Журнал еды за сегодня (/food.json). */
export interface Food {
  src: string;
  date: string;
  items: { time: string; text: string }[];
  recent: { date: string; items: { time: string; text: string }[] }[]; // три дня до сегодняшнего, новые первыми
  history?: { date: string; items: { time: string; text: string }[] }[]; // ?history=1
}

/** Голодание по журналу еды (/fast.json). */
export interface Fast {
  now: string;
  goal: number;
  last?: string;
  hours: number;
  pct: number;
  stage: string;
  next?: string;
  next_in: number;
  windows: { start: string; end: string; kind: "eat" | "fast" | "now" }[];
  fasts: { start: string; end: string; hours: number }[];
  best: number;
  protocol?: string; // выбранный; нет — своя цель goal
  protocols: { id: string; fast: number; window: number; note?: string }[];
  plan?: { start: string; end: string; open: boolean; in: number }; // окно еды по протоколу
}

export interface Shop {
  src: string;
  items: { text: string; group: string }[];
  sections: string[];
  done: number;
}

/** Форма с файлом (витрина): тот же ответ {"ok","message"}, что у send. */
export async function sendForm(path: string, body: FormData): Promise<Result> {
  let r: Response;
  try {
    r = await fetch(path, { method: "POST", headers: API, credentials: "same-origin", body });
  } catch {
    return { ok: false, message: "нет связи с сервером" };
  }
  if (r.status === 401) toGate();
  if ((r.headers.get("Content-Type") || "").includes("application/json")) return (await r.json()) as Result;
  return { ok: false, message: (await r.text()).trim() || `ошибка ${r.status}` };
}

// ── экраны (server/screens.go) ──

export interface DocRef {
  id: string;
  title: string;
  url: string;
  summary?: string;
  type?: string;
  tags?: string[];
  folder?: string;
  snippet?: string;
}

export interface DocPage extends DocRef {
  status?: string;
  pin: boolean;
  path: string;
  words: number;
  created?: string;
  updated?: string;
  html: string;
  crumbs: { label: string; url: string }[];
  links: DocRef[];
  backlinks: DocRef[];
}

export interface ListPage {
  title: string;
  kind: "folder" | "tag" | "search";
  items: DocRef[];
  kids?: { name: string; count: number; url: string }[];
}

export interface CardRaw {
  id: string;
  title: string;
  path: string;
  raw: string;
  ver: string;
}

export interface CardSaved {
  ok: boolean;
  message?: string;
  ver?: string;
  disk?: string;
}

export interface Access {
  fromEnv: boolean;
  logins: { at: string; result: string; ip: string; device: string; ok: boolean }[];
}
