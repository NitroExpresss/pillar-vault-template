import { useState, type FormEvent } from "react";
import type { Board } from "../api";
import type { Act } from "../useAct";

// Календарь: месяц и всё датированное — сроки, столпы, дневник, замеры.
// День раскрывается под сеткой: отметить срок, увидеть день, поставить дело.

type Cal = Board["calendar"];

export function Calendar({ cal, go, act }: { cal: Cal; go: (month: string, day?: string) => void; act: Act }) {
  const [draft, setDraft] = useState("");
  const sel = cal.day;

  function plan(e: FormEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text) return;
    setDraft("");
    void act("/plan", { date: sel.date, text }, "Дело не записалось");
  }

  return (
    <div className="cal">
      <div className="cal-month">
        <div className="cal-nav">
          <button className="cal-arrow" onClick={() => go(cal.prev)} aria-label="Предыдущий месяц">
            <span className="chev back" />
          </button>
          <span className="cal-title">{cal.title}</span>
          <button className="cal-arrow" onClick={() => go(cal.next)} aria-label="Следующий месяц">
            <span className="chev fwd" />
          </button>
          <button className="cal-today" onClick={() => go("")}>
            Сегодня
          </button>
        </div>
        <div className="cal-grid" role="grid" aria-label={cal.title}>
          {cal.week.map((w, i) => (
            <div key={w} className={i >= 5 ? "cal-wd weekend" : "cal-wd"} role="columnheader">
              {w}
            </div>
          ))}
          {cal.weeks.flat().map((d) => {
            const cls = ["cal-day", !d.in && "out", d.today && "today", d.date === sel.date && "sel",
              d.vowBroke ? "broke" : d.vowHeld ? "held" : ""].filter(Boolean).join(" ");
            const tip = [d.due && `сроков: ${d.due}`, d.diary && "запись в дневнике", d.metric && "замер",
              d.vowHeld && `столпов держал: ${d.vowHeld}`, d.vowBroke && `сорвал: ${d.vowBroke}`].filter(Boolean).join(", ");
            return (
              <button key={d.date} className={cls} onClick={() => go(d.date.slice(0, 7), d.date)} title={tip || undefined}
                aria-pressed={d.date === sel.date} aria-label={`${d.day}${tip ? `: ${tip}` : ""}`}>
                <span className="cal-n">{d.day}</span>
                <span className="cal-marks" aria-hidden>
                  {!!d.due && <i className="m-due" />}
                  {d.diary && <i className="m-diary" />}
                  {d.metric && <i className="m-metric" />}
                </span>
              </button>
            );
          })}
        </div>
        <p className="cal-legend">
          <span><i className="m-due" />срок</span>
          <span><i className="m-diary" />дневник</span>
          <span><i className="m-metric" />замер</span>
          <span><i className="m-held" />столпы держал</span>
        </p>
      </div>

      <div className="cal-detail" aria-live="polite">
        <h3 className="cal-label">
          {sel.label}
          {sel.isToday && <span>сегодня</span>}
        </h3>
        {sel.tasks.length > 0 && (
          <ul className="tasks">
            {sel.tasks.map((t) => (
              <li key={t.src + t.text} className={t.done ? "task done" : "task"}>
                <button className="check" aria-label={t.done ? `Открыть снова: ${t.text}` : `Сделано: ${t.text}`}
                  onClick={() => act("/task", { src: t.src, text: t.text, do: t.done ? "open" : "done" })} />
                <a className="task-text" href={t.url}>{t.text}</a>
              </li>
            ))}
          </ul>
        )}
        {sel.vows.length > 0 && (
          <div className="chips">
            {sel.vows.map((v) => (
              <span key={v.name} className={v.value === "✗" ? "chip broke" : "chip held"}>
                {v.name} {v.value === "✗" ? "сорван" : "держал"}
              </span>
            ))}
          </div>
        )}
        {sel.metrics.length > 0 && (
          <div className="chips">
            {sel.metrics.map((m) => (
              <span key={m.name} className="chip">
                {m.name} <b>{m.value}</b>
              </span>
            ))}
          </div>
        )}
        {sel.diary.map((d) => (
          <blockquote key={d.when} className="cal-diary">
            <span>{d.when.slice(11) || d.when}</span>
            {d.text}
          </blockquote>
        ))}
        {sel.tasks.length + sel.vows.length + sel.metrics.length + sel.diary.length === 0 && (
          <p className="quiet">В этот день ничего не записано.</p>
        )}
        <form className="add" onSubmit={plan}>
          <input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder={`Дело на ${sel.label.split(",")[0]}`}
            maxLength={300} enterKeyHint="done" aria-label={`Дело на ${sel.label}`} />
          <button type="submit" disabled={!draft.trim()}>Запланировать</button>
        </form>
      </div>
    </div>
  );
}
