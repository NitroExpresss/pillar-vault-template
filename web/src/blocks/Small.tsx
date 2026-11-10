import { useEffect, useState, type FormEvent } from "react";
import { sendForm, type Board, type BoardTask, type Food, type Shop } from "../api";
import { useToast } from "../toast";
import type { Act } from "../useAct";
import { useData } from "../useData";

// Небольшие виджеты дашборда: дневник, «Что горит», колесо, учёт дел,
// покупки, витрина. У каждого одна работа; запись — через общий act.

export function Diary({ entries, act }: { entries: Board["diary"]; act: Act }) {
  const [text, setText] = useState("");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    if (await act("/diary", { text }, "Запись не сохранилась")) setText("");
  }
  return (
    <div className="diary">
      <form onSubmit={save}>
        <textarea value={text} onChange={(e) => setText(e.target.value)} rows={3} placeholder="Что было сегодня"
          aria-label="Запись в дневник" />
        <button type="submit" className="primary" disabled={!text.trim()}>Записать</button>
      </form>
      {entries.length === 0 ? (
        <p className="quiet">Записей пока нет.</p>
      ) : (
        entries.map((d) => (
          <blockquote key={d.when} className="entry">
            <span>{d.when}</span>
            {d.text}
          </blockquote>
        ))
      )}
    </div>
  );
}

export function Deadlines({ zones, act }: { zones: Board["deadlines"]; act: Act }) {
  if (zones.length === 0) return <p className="quiet">Сроков нет — ничего не горит.</p>;
  return (
    <div className="burn">
      {zones.map((z) => (
        <section key={z.key} className={`burn-zone ${z.key}`}>
          <h3>
            {z.title} <span>{z.items.length}</span>
          </h3>
          <ul className="tasks">
            {z.items.map((t) => (
              <li key={t.src + t.text} className="task">
                <button className="check" aria-label={`Сделано: ${t.text}`} onClick={() => act("/task", { src: t.src, text: t.text, do: "done" })} />
                <a className="task-text" href={t.url}>{t.text}</a>
                <span className="due">{t.when}</span>
                {t.when !== "завтра" && ( /* срок уже завтра — переносить некуда */
                  <button className="remove" aria-label={`На завтра: ${t.text}`} title="На завтра"
                    onClick={() => act("/task", { src: t.src, text: t.text, do: "move" }, "Срок не перенесён")}>
                    На завтра
                  </button>
                )}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

export function Wheel({ spheres }: { spheres: Board["wheel"] }) {
  return (
    <ul className="wheel">
      {spheres.map((s) => (
        <li key={s.name} className={s.low ? "low" : undefined} title={s.goals?.join("; ")}>
          <span className="wheel-name">{s.name}</span>
          <span className="wheel-track" aria-hidden>
            <span style={{ width: `${s.pct}%` }} />
          </span>
          <span className="wheel-score">
            {s.score}
            <small>/{s.max}</small>
          </span>
          {s.cards && (
            <span className="wheel-cards">
              {s.cards.map((c) => (
                <a key={c.url} href={c.url}>{c.title}</a>
              ))}
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}

export function Tasks({ tasks, act }: { tasks: NonNullable<Board["tasks"]>; act: Act }) {
  return (
    <div className="ledger">
      <p className="ledger-sum">
        В работе <b>{tasks.open}</b>, закрыто <b>{tasks.done}</b>
        <span className="meter" aria-label={`Закрыто ${tasks.pct}%`}>
          <span style={{ width: `${tasks.pct}%` }} />
        </span>
      </p>
      {tasks.groups.map((g) => (
        <section key={g.name || "без раздела"}>
          {g.name && <h3>{g.name}</h3>}
          <TaskList items={g.items} act={act} />
        </section>
      ))}
    </div>
  );
}

function TaskList({ items, act }: { items: BoardTask[]; act: Act }) {
  return (
    <ul className="tasks">
      {items.map((t) => (
        <li key={t.src + t.text} className={t.done ? "task done" : "task"}>
          <button className="check" aria-label={t.done ? `Открыть снова: ${t.text}` : `Сделано: ${t.text}`}
            onClick={() => act("/task", { src: t.src, text: t.text, do: t.done ? "open" : "done" })} />
          <a className="task-text" href={t.url}>{t.text}</a>
          {t.due && <span className="due">{t.due}</span>}
        </li>
      ))}
    </ul>
  );
}

// «2026-10-26» → «пн 26 окт»
const dayLabel = (date: string) =>
  new Date(`${date}T12:00:00`).toLocaleDateString("ru-RU", { weekday: "short", day: "numeric", month: "short" });

export function FoodLog({ food, act }: { food: Food; act: Act }) {
  const [text, setText] = useState("");
  const [history, setHistory] = useState(false);
  async function add(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    if (await act("/food", { text }, "Не записалось")) setText("");
  }
  return (
    <div className="food">
      {food.items.length === 0 ? (
        <p className="quiet">Сегодня ещё ничего не записано.</p>
      ) : (
        <ul className="food-list">
          {food.items.map((i, n) => (
            <li key={n}>
              <span className="food-time">{i.time}</span>
              <span className="food-text">{i.text}</span>
              <button className="remove" aria-label={`Убрать: ${i.text}`} onClick={() => act("/food", { do: "remove", time: i.time, text: i.text })}>
                ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <form className="add" onSubmit={add}>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Что съел" maxLength={300} aria-label="Что съел" />
        <button type="submit" disabled={!text.trim()}>Записать</button>
      </form>
      {food.recent.length > 0 && <FoodDays days={food.recent} />}
      <button type="button" className="food-more" aria-expanded={history} onClick={() => setHistory((h) => !h)}>
        {history ? "Скрыть историю" : "История"}
      </button>
      {history && <FoodHistory />}
    </div>
  );
}

/** Все дни журнала, новые первыми — грузится, только когда открыта. */
function FoodHistory() {
  const { data } = useData<Food>("/food.json?history=1");
  if (!data) return <div className="loading" aria-busy />;
  const days = data.history ?? [];
  if (days.length === 0) return <p className="quiet">В журнале пока пусто.</p>;
  return (
    <div className="food-history">
      <FoodDays days={days} />
    </div>
  );
}

/** Дни журнала: заголовок — день, строки — время и что съел. */
function FoodDays({ days }: { days: NonNullable<Food["history"]> }) {
  return (
    <>
      {days.map((d) => (
        <section key={d.date}>
          <h3>{dayLabel(d.date)}</h3>
          <ul className="food-list">
            {d.items.map((i, n) => (
              <li key={n}>
                <span className="food-time">{i.time}</span>
                <span className="food-text">{i.text}</span>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}

export function Shopping({ shop, act }: { shop: Shop; act: Act }) {
  const [text, setText] = useState("");
  const [group, setGroup] = useState(shop.sections[0] ?? "");
  const groups = shop.sections.filter((s) => shop.items.some((i) => i.group === s));

  async function add(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    if (await act("/shop", { text, group }, "Покупка не добавилась")) setText("");
  }

  return (
    <div className="shop">
      {shop.items.length === 0 && <p className="quiet">Всё куплено.</p>}
      {groups.map((g) => (
        <section key={g}>
          <h3>{g}</h3>
          <ul className="tasks">
            {shop.items.filter((i) => i.group === g).map((i) => (
              <li key={i.text} className="task">
                <button className="check" aria-label={`Куплено: ${i.text}`} onClick={() => act("/task", { src: shop.src, text: i.text, do: "done" })} />
                <span className="task-text">{i.text}</span>
              </li>
            ))}
          </ul>
        </section>
      ))}
      <form className="add" onSubmit={add}>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="Что купить" maxLength={300} aria-label="Что купить" />
        {shop.sections.length > 1 && (
          <select value={group} onChange={(e) => setGroup(e.target.value)} aria-label="Раздел">
            {shop.sections.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
        )}
        <button type="submit" disabled={!text.trim()}>Добавить</button>
      </form>
      {shop.done > 0 && <p className="quiet small">Куплено и свёрнуто: {shop.done}</p>}
    </div>
  );
}

export function Gallery({ gallery, reload }: { gallery: NonNullable<Board["gallery"]>; reload: () => Promise<void> }) {
  const [i, setI] = useState(0);
  const n = gallery.slides.length;
  const every = gallery.autoplayMs ?? 7000;
  useEffect(() => {
    if (n < 2 || every <= 0 || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const t = setInterval(() => setI((x) => (x + 1) % n), every);
    return () => clearInterval(t);
  }, [n, every]);
  const s = n > 0 ? gallery.slides[i % n] : null;
  return (
    <div className="gallery">
      {s ? (
        <figure className="slide">
          <img key={s.src} src={`/${s.src}`} alt={s.caption} />
          {(s.caption || s.sub) && (
            <figcaption>
              <b>{s.caption}</b>
              {s.sub && <span>{s.sub}</span>}
            </figcaption>
          )}
        </figure>
      ) : (
        <p className="quiet">Фото пока нет — добавьте первое ниже.</p>
      )}
      {n > 1 && (
        <div className="dots" role="tablist" aria-label="Слайды">
          {gallery.slides.map((x, k) => (
            <button key={x.src + k} role="tab" aria-selected={k === i % n} aria-label={x.caption || `Слайд ${k + 1}`}
              className={k === i % n ? "dot on" : "dot"} onClick={() => setI(k)} />
          ))}
        </div>
      )}
      <GalleryEdit gallery={gallery} current={n > 0 ? i % n : -1} reload={reload} onPick={setI} />
    </div>
  );
}

/** Фото с телефона весит 3–8 МБ: nginx пропускает 4, а в git им не место.
 *  Перед отправкой ужимаем до 2000 px по длинной стороне (JPEG). */
async function shrink(f: File): Promise<File> {
  if (f.type === "image/gif" || f.size < 1_200_000) return f;
  try {
    const img = await createImageBitmap(f, { imageOrientation: "from-image" });
    const k = Math.min(1, 2000 / Math.max(img.width, img.height));
    const c = document.createElement("canvas");
    c.width = Math.round(img.width * k);
    c.height = Math.round(img.height * k);
    c.getContext("2d")!.drawImage(img, 0, 0, c.width, c.height);
    const blob = await new Promise<Blob | null>((res) => c.toBlob(res, "image/jpeg", 0.85));
    return blob ? new File([blob], "photo.jpg", { type: "image/jpeg" }) : f;
  } catch {
    return f; // не вышло ужать — отправим как есть, сервер скажет, если велико
  }
}

function GalleryEdit({ gallery, current, reload, onPick }: {
  gallery: NonNullable<Board["gallery"]>; current: number; reload: () => Promise<void>; onPick: (i: number) => void;
}) {
  const say = useToast();
  const [slot, setSlot] = useState(current >= 0 ? String(current) : "new");
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(""); // «убрать» — в два нажатия: нечаянный тап не сносит слайд
  const picked = slot === "new" ? null : gallery.slides[Number(slot)];

  async function submit(what: "save" | "remove" | "mocks", form?: HTMLFormElement) {
    if ((what === "remove" || what === "mocks") && confirm !== what) {
      setConfirm(what);
      setTimeout(() => setConfirm(""), 3000);
      return;
    }
    setConfirm("");
    const fd = new FormData();
    fd.set("do", what);
    fd.set("slot", slot);
    if (picked) fd.set("src", picked.src);
    if (form) {
      const f = new FormData(form);
      fd.set("caption", `${f.get("caption") ?? ""}`);
      fd.set("sub", `${f.get("sub") ?? ""}`);
      const photo = f.get("photo");
      if (photo instanceof File && photo.size > 0) fd.set("photo", await shrink(photo));
    }
    setBusy(true);
    const res = await sendForm("/gallery", fd);
    setBusy(false);
    if (!res.ok) say(res.message || "Витрина не изменилась");
    if (res.ok && form) form.reset();
    await reload();
  }

  return (
    <details className="gal-edit">
      <summary>Изменить витрину</summary>
      <form key={slot} className="metric-form" onSubmit={(e) => { e.preventDefault(); void submit("save", e.currentTarget); }}>
        <label>
          Слайд
          <select value={slot} onChange={(e) => { setSlot(e.target.value); if (e.target.value !== "new") onPick(Number(e.target.value)); }}>
            {gallery.slides.map((x, k) => (
              <option key={x.src + k} value={k}>{k + 1}. {x.caption || "без подписи"}</option>
            ))}
            <option value="new">Новый слайд</option>
          </select>
        </label>
        <label>
          Фото
          <input type="file" name="photo" accept="image/*" />
        </label>
        <label>
          Подпись
          <input name="caption" maxLength={80} defaultValue={picked?.caption ?? ""} placeholder="Форма — 85 кг" />
        </label>
        <label>
          Строка под ней
          <input name="sub" maxLength={120} defaultValue={picked?.sub ?? ""} placeholder="цель первого этапа" />
        </label>
        <div className="form-bar">
          <button type="submit" className="primary" disabled={busy}>{busy ? "Сохраняю" : "Сохранить"}</button>
          {picked && (
            <button type="button" className={confirm === "remove" ? "ghost armed" : "ghost"} onClick={() => submit("remove")}>
              {confirm === "remove" ? "Точно убрать?" : "Убрать слайд"}
            </button>
          )}
          {gallery.mocks > 0 && (
            <button type="button" className={confirm === "mocks" ? "ghost armed" : "ghost"} onClick={() => submit("mocks")}>
              {confirm === "mocks" ? "Убрать все примеры?" : `Убрать примеры (${gallery.mocks})`}
            </button>
          )}
          <span className="quiet">Без фото меняются только подписи. Большие фото ужимаются перед отправкой.</span>
        </div>
      </form>
    </details>
  );
}

/** Важные заметки: закреплённые карточки (pin: true). Открепить — тут же. */
export function Pins({ pins, act }: { pins: Board["pins"]; act: Act }) {
  if (pins.length === 0)
    return <p className="quiet">Закрепите карточку кнопкой «Закрепить» на её странице — она появится здесь.</p>;
  return (
    <ul className="note-pins">
      {pins.map((p) => (
        <li key={p.id} className="note-pin">
          <a href={p.url}>
            <span className="note-pin-title">{p.title}</span>
            {p.summary && <span className="note-pin-text">{p.summary}</span>}
          </a>
          <button className="remove" aria-label={`Открепить «${p.title}»`} title="Открепить"
            onClick={() => act("/pin", { id: p.id, on: "0" }, "Не откреплено")}>×</button>
        </li>
      ))}
    </ul>
  );
}
