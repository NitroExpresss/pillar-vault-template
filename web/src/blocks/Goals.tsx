import { useEffect, useRef, useState, type FormEvent, type PointerEvent } from "react";
import type { Goal, GoalKR } from "../api";
import { plural } from "../lib";
import type { Act } from "../useAct";

// Цели как OKR: у каждой — график «пройдено пути» по ключевым результатам
// на одной оси 0–100% (вес вниз и книги вверх сравнимы только так), пунктир
// идеального темпа от старта к сроку и линия «сегодня». Линия ниже пунктира —
// результат отстаёт. Под графиком — сами числа: откуда, где сейчас, куда.

const NEW_GOAL = `/new?${new URLSearchParams({ type: "goal", tag: "цели" })}`;
const MONTHS = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
const day = (iso: string) => Date.parse(iso + "T00:00:00Z") / 86400000;
const short = (iso: string) => `${Number(iso.slice(8, 10))} ${MONTHS[Number(iso.slice(5, 7)) - 1]}`;
const fmt = (n: number) => String(Math.round(n * 10) / 10).replace(".", ",");

const PACE: Record<Goal["pace"], (g: Goal) => string> = {
  ahead: (g) => `опережает на ${fmt(g.gap)} п.п.`,
  on: () => "по графику",
  behind: (g) => `отстаёт на ${fmt(-g.gap)} п.п.`,
  done: () => "достигнута",
  nodeadline: () => "без срока",
  nodata: () => "нет показаний",
};

export function Goals({ goals, today, act }: { goals: Goal[]; today: string; act: Act }) {
  if (goals.length === 0)
    return (
      <div className="empty">
        <p className="quiet">Целей пока нет. Заведите цель — срок и результаты добавляются прямо здесь.</p>
        <a className="ghost" href={NEW_GOAL}>+ Новая цель</a>
      </div>
    );
  return (
    <div className="goals">
      {goals.map((g) => (
        <GoalCard key={g.id} g={g} today={today} act={act} />
      ))}
      <a className="ghost new-goal" href={NEW_GOAL}>+ Новая цель</a>
    </div>
  );
}

function GoalCard({ g, today, act }: { g: Goal; today: string; act: Act }) {
  const left = g.deadline
    ? g.daysLeft >= 0
      ? `${g.daysLeft} ${plural(g.daysLeft, "день", "дня", "дней")} до ${short(g.deadline)}`
      : `срок прошёл ${short(g.deadline)}`
    : "срок не задан";
  return (
    <article className="goal-card">
      <header className="goal-head">
        <a href={g.url}>{g.title}</a>
        <span className={`pace ${g.pace}`}>{PACE[g.pace](g)}</span>
      </header>
      <p className="goal-meta">
        {left}. Пройдено {fmt(g.pct)}%{g.deadline ? `, времени прошло ${fmt(g.timePct)}%` : ""}
      </p>
      {g.krs.some((k) => k.points.length > 0) && <GoalChart g={g} today={today} />}
      <ul className="krs">
        {g.krs.map((k, i) => (
          <li key={k.name}>
            <span className="kr-swatch" style={{ background: `var(--goal-${(i % 4) + 1})` }} aria-hidden />
            <span className="kr-name">{k.name}</span>
            <span className="kr-nums">
              {k.points.length === 0
                ? `цель ${fmt(k.target)}${k.unit ? " " + k.unit : ""}, показаний нет`
                : `${fmt(k.start)} → ${fmt(k.current)} из ${fmt(k.target)}${k.unit ? " " + k.unit : ""}`}
            </span>
            <span className="kr-bar" aria-hidden>
              <span style={{ width: `${Math.max(0, Math.min(100, k.pct))}%` }} />
            </span>
            <span className="kr-pct">{fmt(k.pct)}%</span>
          </li>
        ))}
        {g.tasks[1] > 0 && (
          <li>
            <span className="kr-swatch tasks" aria-hidden />
            <span className="kr-name">Дела</span>
            <span className="kr-nums">
              {g.tasks[0]} из {g.tasks[1]}
            </span>
            <span className="kr-bar" aria-hidden>
              <span style={{ width: `${(100 * g.tasks[0]) / g.tasks[1]}%` }} />
            </span>
            <span className="kr-pct">{Math.round((100 * g.tasks[0]) / g.tasks[1])}%</span>
          </li>
        )}
      </ul>
      <GoalForms g={g} today={today} act={act} />
    </article>
  );
}

/** Правка цели без markdown: показание, срок, новый результат (POST /goal). */
function GoalForms({ g, today, act }: { g: Goal; today: string; act: Act }) {
  const [open, setOpen] = useState<"" | "value" | "deadline" | "kr">("");
  const own = g.krs.filter((k) => !k.from); // показания из журнала замеров пишутся «Замером»
  const toggle = (f: typeof open) => setOpen((o) => (o === f ? "" : f));

  async function submit(e: FormEvent<HTMLFormElement>, fields: Record<string, string>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    for (const [k, v] of f.entries()) fields[k] = String(v);
    if (await act("/goal", { id: g.id, ...fields }, "Цель не изменилась")) setOpen("");
  }

  return (
    <div className="goal-forms">
      <div className="goal-acts">
        {own.length > 0 && (
          <button className={open === "value" ? "ghost on" : "ghost"} onClick={() => toggle("value")}>Записать показание</button>
        )}
        <button className={open === "deadline" ? "ghost on" : "ghost"} onClick={() => toggle("deadline")}>
          {g.deadline ? "Срок" : "Задать срок"}
        </button>
        <button className={open === "kr" ? "ghost on" : "ghost"} onClick={() => toggle("kr")}>+ Результат</button>
      </div>
      {open === "value" && (
        <form className="metric-form" onSubmit={(e) => submit(e, { do: "value" })}>
          <label>
            Дата
            <input type="date" name="date" defaultValue={today} max={today} required />
          </label>
          {own.map((k) => (
            <label key={k.col}>
              {k.name}{k.unit ? `, ${k.unit}` : ""}
              <input name={`v${k.col}`} inputMode="decimal" autoComplete="off"
                placeholder={k.points.length ? `было ${String(k.current).replace(".", ",")}` : `цель ${k.target}`} />
            </label>
          ))}
          <div className="form-bar">
            <button type="submit" className="primary">Записать</button>
            {g.krs.some((k) => k.from) && <span className="quiet">Вес и замеры — через «Записать замер» в «Теле».</span>}
          </div>
        </form>
      )}
      {open === "deadline" && (
        <form className="metric-form" onSubmit={(e) => submit(e, { do: "deadline" })}>
          <label>
            Срок
            <input type="date" name="deadline" defaultValue={g.deadline ?? ""} min={g.start} required />
          </label>
          <div className="form-bar">
            <button type="submit" className="primary">Сохранить срок</button>
            {g.deadline && (
              <button type="button" className="ghost" onClick={() => act("/goal", { id: g.id, do: "deadline", deadline: "" }).then((ok) => ok && setOpen(""))}>
                Снять срок
              </button>
            )}
          </div>
        </form>
      )}
      {open === "kr" && (
        <form className="metric-form" onSubmit={(e) => submit(e, { do: "kr" })}>
          <label>
            Результат
            <input name="name" required maxLength={40} placeholder="Подушка" autoComplete="off" />
          </label>
          <label>
            Единица
            <input name="unit" maxLength={20} placeholder="₽ тыс" autoComplete="off" />
          </label>
          <label>
            Цель
            <input name="target" inputMode="decimal" required placeholder="300" autoComplete="off" />
          </label>
          <div className="form-bar">
            <button type="submit" className="primary">Добавить результат</button>
            <span className="quiet">Первое показание — старт, от него считается путь.</span>
          </div>
        </form>
      )}
    </div>
  );
}

const H = 190;
const PAD = { l: 34, t: 12, b: 24 };
/** Ширина подписи конца линии: на телефоне подписей нет — имена в списке под графиком. */
const labelSpace = (w: number, labels: string[]) =>
  w < 480 ? 14 : Math.min(w * 0.34, 28 + 7.2 * Math.max(0, ...labels.map((l) => l.length)));

/** Ширина графика — по контейнеру: подписи остаются читаемыми на телефоне. */
function useWidth() {
  const ref = useRef<HTMLDivElement>(null);
  const [w, setW] = useState(0); // 0 — ещё не измерили: SVG не должен распирать контейнер
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setW(Math.max(260, Math.round(e.contentRect.width))));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, w] as const;
}

/** Значение результата на дату: последнее показание не позже неё. */
const at = (k: GoalKR, d: number) => {
  let hit: GoalKR["points"][number] | undefined;
  for (const p of k.points) if (day(p.date) <= d) hit = p;
  return hit;
};

function GoalChart({ g, today }: { g: Goal; today: string }) {
  const [ref, w] = useWidth();
  const [hover, setHover] = useState<number | null>(null);
  const live = g.krs.map((k, i) => ({ k, i })).filter(({ k }) => k.points.length > 0);
  const last = Math.max(...live.flatMap(({ k }) => k.points.map((p) => day(p.date))));
  const d0 = Math.min(day(g.start), ...live.map(({ k }) => day(k.points[0].date)));
  const d1 = Math.max(g.deadline ? day(g.deadline) : day(today), day(today), last);
  const span = Math.max(1, d1 - d0);
  const PADR = labelSpace(w, live.map(({ k }) => `${k.name} ${fmt(k.pct)}%`));
  const x = (d: number) => PAD.l + ((d - d0) / span) * (w - PAD.l - PADR);
  const y = (pct: number) => PAD.t + (1 - Math.max(-10, Math.min(110, pct)) / 100) * (H - PAD.t - PAD.b);
  const now = day(today);

  // подписи концов линий не должны наезжать друг на друга
  const ends = live
    .map(({ k, i }) => ({ i, k, yy: y(k.points[k.points.length - 1].pct) }))
    .sort((a, b) => a.yy - b.yy);
  for (let j = 1; j < ends.length; j++) ends[j].yy = Math.max(ends[j].yy, ends[j - 1].yy + 14);

  function move(e: PointerEvent<SVGRectElement>) {
    const r = e.currentTarget.getBoundingClientRect();
    const d = d0 + ((e.clientX - r.left) / r.width) * span;
    // к ближайшему показанию — подсказка о настоящей точке, а не о пустом месте
    let best = d, dist = Infinity;
    for (const { k } of live)
      for (const p of k.points) {
        const dd = Math.abs(day(p.date) - d);
        if (dd < dist) (dist = dd), (best = day(p.date));
      }
    setHover(best);
  }

  const tip = hover === null ? null : live.map(({ k, i }) => ({ k, i, p: at(k, hover) })).filter((t) => t.p);
  const tipDate = hover === null ? "" : new Date(hover * 86400000).toISOString().slice(0, 10);

  return (
    <div className="goal-chart" ref={ref}>
      {w > 0 && <svg width={w} height={H} role="img" aria-label={`${g.title}: пройдено ${fmt(g.pct)}% пути`}>
        {[0, 50, 100].map((v) => (
          <g key={v}>
            <line className="gc-grid" x1={PAD.l} x2={w - PADR} y1={y(v)} y2={y(v)} />
            <text className="gc-axis" x={PAD.l - 6} y={y(v) + 4} textAnchor="end">{v}%</text>
          </g>
        ))}
        <text className="gc-axis" x={PAD.l} y={H - 6}>{short(new Date(d0 * 86400000).toISOString().slice(0, 10))}</text>
        {g.deadline && (
          <text className="gc-axis" x={x(day(g.deadline))} y={H - 6} textAnchor="end">срок {short(g.deadline)}</text>
        )}
        {g.deadline && (
          <line className="gc-ideal" x1={x(day(g.start))} y1={y(0)} x2={x(day(g.deadline))} y2={y(100)}>
            <title>Идеальный темп: равномерно от старта к сроку</title>
          </line>
        )}
        {now > d0 && now < d1 && (
          <g>
            <line className="gc-today" x1={x(now)} x2={x(now)} y1={PAD.t} y2={H - PAD.b} />
            <text className="gc-axis" x={x(now)} y={PAD.t - 2} textAnchor="middle">сегодня</text>
          </g>
        )}
        {live.map(({ k, i }) => (
          <polyline key={k.name} className="gc-line" style={{ stroke: `var(--goal-${(i % 4) + 1})` }}
            points={k.points.map((p) => `${x(day(p.date))},${y(p.pct)}`).join(" ")} />
        ))}
        {live.map(({ k, i }) => {
          const p = k.points[k.points.length - 1];
          return <circle key={k.name} className="gc-dot" cx={x(day(p.date))} cy={y(p.pct)} r={4.5} style={{ fill: `var(--goal-${(i % 4) + 1})` }} />;
        })}
        {w >= 480 && ends.map(({ k, i, yy }) => {
          const p = k.points[k.points.length - 1];
          return (
            <g key={k.name}>
              <rect className="gc-key" x={w - PADR + 8} y={yy - 5} width={8} height={3} rx={1.5}
                style={{ fill: `var(--goal-${(i % 4) + 1})` }} />
              <text className="gc-label" x={w - PADR + 20} y={yy}>
                {k.name} {fmt(p.pct)}%
              </text>
            </g>
          );
        })}
        {hover !== null && <line className="gc-cross" x1={x(hover)} x2={x(hover)} y1={PAD.t} y2={H - PAD.b} />}
        <rect className="gc-hit" x={PAD.l} y={0} width={w - PAD.l - PADR} height={H} onPointerMove={move}
          onPointerDown={move} onPointerLeave={() => setHover(null)} />
      </svg>}
      {tip && tip.length > 0 && (
        <div className="gc-tip" style={{ left: Math.min(Math.max(0, x(hover!) - 80), w - 170) }} role="status">
          <b>{short(tipDate)}</b>
          {tip.map(({ k, i, p }) => (
            <span key={k.name}>
              <i style={{ background: `var(--goal-${(i % 4) + 1})` }} />
              {k.name} {fmt(p!.value)}
              {k.unit ? " " + k.unit : ""}, {fmt(p!.pct)}%
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
