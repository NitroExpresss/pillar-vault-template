import { useEffect, useState, type FormEvent } from "react";
import type { Fast as FastData } from "../api";
import type { Act } from "../useAct";

// Голодание: кольцо — сколько идёт голод против цели протокола, окно еды по
// протоколу, ниже — неделя окон по суткам: когда ел, когда голодал, где
// плановое окно. Считает сервер (server/fast.go) по журналу еды; протоколы —
// таблица карточки guides/intermittent-fasting-keto. Здесь только тикают
// минуты до следующего опроса.

const DAY = 24 * 3600 * 1000;
const WD = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];

// 18.4 ч → «18 ч 24 мин»; дробь часа — минутами
function dur(h: number) {
  const m = Math.max(0, Math.round(h * 60));
  const hh = Math.floor(m / 60);
  const mm = m % 60;
  return hh === 0 ? `${mm} мин` : mm === 0 ? `${hh} ч` : `${hh} ч ${mm} мин`;
}

const clock = (iso: string) => new Date(iso).toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });

export function Fast({ fast, act }: { fast: FastData; act: Act }) {
  // часы между опросами сервера — по часам телефона
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);

  if (!fast.last) {
    return (
      <div className="fast">
        <Protocols fast={fast} act={act} />
        <p className="quiet">Голодание считается по журналу еды — запишите, что ели, и время пойдёт.</p>
      </div>
    );
  }

  const drift = Math.max(0, (now - Date.parse(fast.now)) / 3600_000);
  const h = fast.hours + drift;
  const share = h / fast.goal;
  const R = 52;
  const C = 2 * Math.PI * R;
  const done = share >= 1;
  const nextIn = fast.next ? fast.next_in - drift : 0;
  const plan = fast.plan;
  const planIn = plan ? plan.in - drift : 0;

  return (
    <div className="fast">
      <Protocols fast={fast} act={act} />
      <div className="fast-top">
        <svg className={done ? "fast-ring done" : "fast-ring"} viewBox="0 0 120 120" role="img"
          aria-label={`Голод ${dur(h)} из ${fast.goal} ч`}>
          <circle className="fast-track" cx="60" cy="60" r={R} />
          <circle className="fast-arc" cx="60" cy="60" r={R} strokeDasharray={`${C * Math.min(share, 1)} ${C}`} />
          {share > 1 && <circle className="fast-over" cx="60" cy="60" r={R - 9} strokeDasharray={`${C * Math.min(share - 1, 1) * ((R - 9) / R)} ${C}`} />}
          <text x="60" y="56" className="fast-h">{Math.floor(h)}:{String(Math.floor((h % 1) * 60)).padStart(2, "0")}</text>
          <text x="60" y="76" className="fast-of">из {fast.goal} ч · {Math.round(share * 100)}%</text>
        </svg>
        <div className="fast-facts">
          {plan && planIn > 0 && (
            <p className={plan.open ? "fast-plan-line open" : "fast-plan-line"}>
              {plan.open ? `Окно еды открыто до ${plan.end} — ещё ${dur(planIn)}` : `Окно еды ${plan.start}–${plan.end} — через ${dur(planIn)}`}
            </p>
          )}
          <p className="fast-stage">{fast.stage}</p>
          {fast.next && nextIn > 0 && <p className="quiet small">до «{fast.next}» — {dur(nextIn)}</p>}
          <p className="quiet small">последняя еда — {clock(fast.last)}{Date.now() - Date.parse(fast.last) > DAY / 2 ? `, ${new Date(fast.last).toLocaleDateString("ru-RU", { weekday: "short", day: "numeric" })}` : ""}</p>
          <p className="quiet small">{done ? "Цель взята" : `до цели ${dur(fast.goal - h)}`}</p>
          {fast.fasts.length > 0 && (
            <p className="fast-past" aria-label="Прошлые голодания">
              {fast.fasts.slice(0, 5).map((f) => (
                <span key={f.end} className={f.hours >= fast.goal ? "hit" : undefined} title={`${clock(f.start)} → ${clock(f.end)}`}>
                  {Math.round(f.hours)} ч
                </span>
              ))}
            </p>
          )}
        </div>
      </div>
      <Week fast={fast} now={now} />
    </div>
  );
}

/** Выбор протокола: кнопки из таблицы гайда, своя цель, начало окна еды. */
function Protocols({ fast, act }: { fast: FastData; act: Act }) {
  const [open, setOpen] = useState(false);
  const [goal, setGoal] = useState("");
  const cur = fast.protocols.find((p) => p.id === fast.protocol);
  const save = (fields: Record<string, string>) => act("/fast", fields, "Протокол не сохранился");

  function own(e: FormEvent) {
    e.preventDefault();
    if (goal.trim()) void save({ protocol: "", goal }).then((ok) => ok && setGoal(""));
  }

  return (
    <div className="fast-proto">
      <button type="button" className="fast-proto-btn" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        Протокол: <b>{cur ? cur.id : `своя цель ${fast.goal} ч`}</b>
        {cur && cur.window > 0 && <span className="quiet"> · {cur.fast}:{cur.window}</span>}
        <span className={open ? "chev open" : "chev"} aria-hidden />
      </button>
      {cur && cur.window > 0 && (
        <label className="fast-window">
          окно еды с
          <input type="time" defaultValue={fast.plan?.start ?? ""} key={fast.plan?.start ?? ""}
            onBlur={(e) => e.target.value !== (fast.plan?.start ?? "") && void save({ protocol: cur.id, window: e.target.value })} />
        </label>
      )}
      {open && (
        <div className="fast-proto-list">
          <div className="fast-chips" role="radiogroup" aria-label="Протокол голодания">
            {fast.protocols.map((p) => (
              <button key={p.id} type="button" role="radio" aria-checked={p.id === fast.protocol}
                className={p.id === fast.protocol ? "on" : undefined} title={p.note}
                onClick={() => void save({ protocol: p.id }).then((ok) => ok && setOpen(false))}>
                {p.id}
              </button>
            ))}
          </div>
          {cur?.note && <p className="quiet small">{cur.note}</p>}
          <form className="add" onSubmit={own}>
            <input value={goal} onChange={(e) => setGoal(e.target.value)} inputMode="decimal" placeholder="Своя цель, часов"
              aria-label="Своя цель голодания, часов" />
            <button type="submit" disabled={!goal.trim()}>Задать</button>
          </form>
          <p className="quiet small">Протоколы и как они работают — <a href="/doc/guides/intermittent-fasting-keto">Интервальное голодание и кето</a></p>
        </div>
      )}
    </div>
  );
}

/** Неделя по суткам: строка — день, полоса — 24 часа; еда и голод цветом. */
function Week({ fast, now }: { fast: FastData; now: number }) {
  // плановое окно еды протокола — пунктиром в каждом дне (через полночь — двумя кусками)
  const cur = fast.protocols.find((p) => p.id === fast.protocol);
  const planSegs: [number, number][] = [];
  if (cur && cur.window > 0 && fast.plan) {
    const [hh, mm] = fast.plan.start.split(":").map(Number);
    const a = (hh + mm / 60) / 24;
    const b = a + cur.window / 24;
    planSegs.push([a, Math.min(b, 1)]);
    if (b > 1) planSegs.push([0, b - 1]);
  }
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const days = Array.from({ length: 7 }, (_, i) => today.getTime() - i * DAY);
  const wins = fast.windows.map((w) => ({
    a: Date.parse(w.start),
    b: w.kind === "now" ? now : Date.parse(w.end),
    kind: w.kind,
  }));
  return (
    <div className="fast-week" aria-label="Окна еды и голода за неделю">
      {days.map((d0) => {
        const d1 = d0 + DAY;
        const day = new Date(d0);
        return (
          <div key={d0} className="fast-day">
            <span className="fast-dl">{WD[day.getDay()]} {day.getDate()}</span>
            <span className="fast-bar">
              {planSegs.map(([a, b], i) => (
                <span key={`p${i}`} className="fast-seg fast-plan" style={{ left: `${a * 100}%`, width: `${(b - a) * 100}%` }} />
              ))}
              {wins
                .filter((w) => w.b > d0 && w.a < d1)
                .map((w, i) => {
                  const a = Math.max(w.a, d0);
                  const b = Math.min(w.b, d1);
                  return (
                    <span key={i} className={`fast-seg fast-${w.kind}`}
                      style={{ left: `${((a - d0) / DAY) * 100}%`, width: `${((b - a) / DAY) * 100}%` }} />
                  );
                })}
            </span>
          </div>
        );
      })}
      <div className="fast-day fast-axis" aria-hidden>
        <span className="fast-dl" />
        <span className="fast-bar">
          {[0, 6, 12, 18, 24].map((t) => (
            <span key={t} style={{ left: `${(t / 24) * 100}%` }}>{t}</span>
          ))}
        </span>
      </div>
      <p className="fast-legend quiet small">
        <span className="fast-key fast-eat" /> еда <span className="fast-key fast-fast" /> голод <span className="fast-key fast-now" /> сейчас
        {planSegs.length > 0 && <><span className="fast-key fast-plan" /> окно по протоколу</>}
      </p>
    </div>
  );
}
