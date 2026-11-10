import { useMemo } from "react";
import { send, type Vow, type Vows } from "../api";
import { plural } from "../lib";
import { useToast } from "../toast";

// Столпы дня — сад. Стебель растёт со стриком (за месяц — в полный рост),
// листья прибавляются каждые три дня, «держу» распускает цветок, «сорвал»
// клонит стебель. Колышек рядом — рекорд: до какой высоты уже доходил.
// Нажатие на растение — «держу»; сорвать — отдельной кнопкой, не случайно.

const FULL = 30;
const GROUND = 128; // земля в координатах рисунка
const SOIL = "アイウエオカキクケコサシスセソタチツテト0123456789";

/** Высота стебля в единицах рисунка: даже нулевой стрик — заметный росток. */
const stemFor = (days: number) => 34 + (74 * Math.min(days, FULL)) / FULL;

export function Garden({ data, set, reload }: { data: Vows; set: (v: Vows) => void; reload: () => Promise<void> }) {
  const say = useToast();
  // почва — строка символов матрицы: сад растёт прямо из кода
  const soil = useMemo(() => Array.from({ length: 64 }, () => SOIL[(Math.random() * SOIL.length) | 0]).join(""), []);

  async function mark(v: Vow, m: "✓" | "✗") {
    navigator.vibrate?.(12);
    const next = v.mark === m ? "" : m; // повторное нажатие снимает отметку — как на сервере
    set({ ...data, vows: data.vows.map((x) => (x.name === v.name ? { ...x, mark: next } : x)) });
    const res = await send("/vow", { name: v.name, mark: m });
    if (!res.ok) say(res.message || "Отметка не сохранилась");
    await reload();
  }

  return (
    <div className="garden">
      <div className="beds" role="group" aria-label="Столпы дня">
        {data.vows.map((v) => {
          const state = v.mark === "✓" ? "held" : v.mark === "✗" ? "broke" : "open";
          return (
            <div key={v.name} className={`bed ${state}`}>
              <button
                className="plant-btn"
                onClick={() => mark(v, "✓")}
                aria-pressed={state === "held"}
                title={v.rule}
                aria-label={`${v.name}: ${state === "held" ? "держу, снять отметку" : "отметить, что держу"}`}
              >
                <Plant streak={v.streak} record={v.record} state={state} />
              </button>
              <div className="bed-name">{v.name}</div>
              <div className="bed-streak" title={`Рекорд — ${v.record}`}>
                <b>{v.streak}</b> {plural(v.streak, "день", "дня", "дней")}
              </div>
              <button className={state === "broke" ? "bed-break on" : "bed-break"} onClick={() => mark(v, "✗")} aria-pressed={state === "broke"}>
                {state === "broke" ? "Сорван" : "Сорвал"}
              </button>
            </div>
          );
        })}
      </div>
      <div className="soil" aria-hidden>
        {soil}
      </div>
    </div>
  );
}

function Plant({ streak, record, state }: { streak: number; record: number; state: "open" | "held" | "broke" }) {
  const top = GROUND - stemFor(streak);
  const stake = GROUND - stemFor(record);
  // сорванный клонится: верхушка уходит вбок и вниз
  const tipX = state === "broke" ? 56 : 40;
  const tipY = state === "broke" ? top + 14 : top;
  const stem = `M40 ${GROUND} Q ${state === "broke" ? 34 : 37} ${(GROUND + top) / 2} ${tipX} ${tipY}`;
  const leaves = Math.min(6, 1 + Math.ceil(streak / 3)); // первый лист — сразу, как у живого ростка
  const span = GROUND - top;

  return (
    <svg className="plant" viewBox="0 0 80 140" aria-hidden>
      {record > streak && (
        <g className="stake">
          <line x1="66" y1={GROUND} x2="66" y2={stake} />
          <line x1="62" y1={stake} x2="70" y2={stake} />
        </g>
      )}
      <path className="stem" d={stem} pathLength={1} />
      {Array.from({ length: leaves }, (_, i) => {
        const y = GROUND - ((i + 1) * span) / (leaves + 1);
        const left = i % 2 === 0;
        const d = left ? `M39 ${y} q -13 -9 -20 1 q 11 6 20 -1z` : `M41 ${y} q 13 -9 20 1 q -11 6 -20 -1z`;
        return <path key={i} className="leaf" d={d} style={{ animationDelay: `${300 + i * 80}ms` }} />;
      })}
      {state === "held" ? (
        <g className="bloom" transform={`translate(${tipX} ${tipY})`}>
          {[0, 60, 120, 180, 240, 300].map((a) => (
            <ellipse key={a} className="petal" rx="4.6" ry="8.5" cy="-8.5" transform={`rotate(${a})`} />
          ))}
          <circle className="core" r="4.4" />
        </g>
      ) : (
        <ellipse className={state === "broke" ? "bud wilted" : "bud"} cx={tipX} cy={tipY - 4} rx="4.2" ry="6.5" transform={state === "broke" ? `rotate(50 ${tipX} ${tipY})` : undefined} />
      )}
    </svg>
  );
}
