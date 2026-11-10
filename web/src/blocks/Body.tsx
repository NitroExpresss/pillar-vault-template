import { useState, type FormEvent } from "react";
import type { Board } from "../api";
import type { Act } from "../useAct";

// Тело: путь к цели, графики и «+ замер». Графики рисует сервер (charts.go)
// — числа из карточки, подписи экранированы; здесь их только красят токены.

type BodyData = NonNullable<Board["body"]>;

const fmt = (n: number) => String(Math.round(n * 10) / 10).replace(".", ",");

export function Body({ body, today, act }: { body: BodyData; today: string; act: Act }) {
  const [open, setOpen] = useState(false);
  const [date, setDate] = useState(today);
  const [vals, setVals] = useState<Record<number, string>>({});
  const g = body.goal;

  async function save(e: FormEvent) {
    e.preventDefault();
    const fields: Record<string, string> = { do: "add", date, back: "/#metrics" };
    for (const f of body.fields) fields[`v${f.idx}`] = vals[f.idx] ?? "";
    if (await act("/metric", fields, "Замер не записался")) {
      setVals({});
      setOpen(false);
    }
  }

  return (
    <div className="body">
      {body.demo && (
        <p className="note">
          Пока здесь примерные замеры. Впишите свои в <a href={body.url}>карточку</a> — графики построятся по ним.
        </p>
      )}
      {g && (
        <a className="goal" href={body.url} aria-label={`Путь к цели: пройдено ${g.pct}%`}>
          <span className="goal-now">
            <b>{fmt(g.now)}</b> кг, до цели {fmt(g.left)} кг
          </span>
          <span className="goal-rate">{g.rate > 0 ? `−${fmt(g.rate)} кг в неделю` : "Темпа пока нет"}</span>
          <span className="goal-track">
            <span style={{ width: `${g.pct}%` }} />
          </span>
          <span className="goal-ends">
            <span>старт {fmt(g.start)}</span>
            <span>цель {fmt(g.target)}</span>
          </span>
        </a>
      )}
      <div className="charts">
        {body.charts.map((svg, i) => (
          <div key={i} dangerouslySetInnerHTML={{ __html: svg }} />
        ))}
      </div>
      {open ? (
        <form className="metric-form" onSubmit={save}>
          <label>
            Дата
            <input type="date" value={date} max={today} onChange={(e) => setDate(e.target.value)} />
          </label>
          {body.fields.map((f) => (
            <label key={f.idx}>
              {f.name}
              <input inputMode="decimal" autoComplete="off" placeholder={f.last ? `было ${f.last}` : ""}
                value={vals[f.idx] ?? ""} onChange={(e) => setVals((v) => ({ ...v, [f.idx]: e.target.value }))} />
            </label>
          ))}
          <div className="form-bar">
            <button type="submit" className="primary">Записать замер</button>
            <button type="button" className="ghost" onClick={() => setOpen(false)}>Отмена</button>
            <span className="quiet">Пустое поле — эта мерка не менялась.</span>
          </div>
        </form>
      ) : (
        <button className="ghost add-metric" onClick={() => setOpen(true)}>
          Записать замер
        </button>
      )}
    </div>
  );
}
