import { useState, type FormEvent } from "react";
import { send, type Today as TodayData } from "../api";
import { useToast } from "../toast";
import { useData } from "../useData";

// Дела дня. Отметка срабатывает сразу: строка зачёркивается и уходит, не
// дожидаясь сервера; отказ сервера возвращает её и называет причину.

type Leaving = "done" | "gone";

export function Today() {
  const { data, set, reload } = useData<TodayData>("/today.json");
  const say = useToast();
  const [leaving, setLeaving] = useState<Record<string, Leaving>>({});
  // дело уже в списке, но сервер его ещё не записал: отметить его сейчас —
  // значит обогнать запись и получить «этого дела нет на сегодня»
  const [saving, setSaving] = useState<Set<string>>(new Set());
  const [draft, setDraft] = useState("");

  async function act(text: string, how: "done" | "remove" | "later") {
    setLeaving((l) => ({ ...l, [text]: how === "done" ? "done" : "gone" }));
    navigator.vibrate?.(12);
    const res = await send("/today", { text, do: how });
    if (!res.ok) say(res.message || "Не получилось");
    setLeaving(({ [text]: _, ...rest }) => rest);
    await reload();
  }

  async function add(raw: string, how: "add" | "new") {
    const text = raw.trim();
    if (!text || !data) return;
    if (!data.tasks.some((t) => t.text === text)) {
      set({ ...data, tasks: [...data.tasks, { text, url: "" }], pick: data.pick.filter((p) => p !== text) });
    }
    setSaving((s) => new Set(s).add(text));
    const res = await send("/today", { text, do: how });
    if (!res.ok) say(res.message || "Дело не добавилось");
    await reload();
    setSaving((s) => {
      const next = new Set(s);
      next.delete(text);
      return next;
    });
  }

  function submitNew(e: FormEvent) {
    e.preventDefault();
    const text = draft;
    setDraft("");
    void add(text, "new");
  }

  if (!data) return <div className="loading" aria-busy />;
  return (
    <div className="today">
      {data.tasks.length === 0 ? (
        <p className="quiet">Дел на сегодня нет. Выберите из списка или впишите своё.</p>
      ) : (
        <ul className="tasks">
          {data.tasks.map((t) => {
            const busy = !!leaving[t.text] || saving.has(t.text);
            const cls = ["task", leaving[t.text], saving.has(t.text) && "saving", t.late && "late"].filter(Boolean).join(" ");
            return (
              <li key={t.text} className={cls}>
                <button className="check" onClick={() => act(t.text, "done")} disabled={busy} aria-label={`Сделано: ${t.text}`} />
                {t.url ? (
                  <a className="task-text" href={t.url}>
                    {t.text}
                  </a>
                ) : (
                  <span className="task-text">{t.text}</span>
                )}
                {t.due && <span className="due">{t.late ? `просрочено, ${t.due}` : t.due}</span>}
                <button className="remove" onClick={() => act(t.text, "later")} disabled={busy} aria-label={`На завтра: ${t.text}`} title="На завтра">
                  На завтра
                </button>
                <button className="remove" onClick={() => act(t.text, "remove")} disabled={busy} aria-label={`Убрать из дня: ${t.text}`}>
                  Убрать
                </button>
              </li>
            );
          })}
        </ul>
      )}
      <form className="add" onSubmit={submitNew}>
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Новое дело на сегодня"
          maxLength={300}
          enterKeyHint="done"
          aria-label="Новое дело на сегодня"
        />
        <button type="submit" disabled={!draft.trim()}>
          Добавить
        </button>
      </form>
      {data.pick.length > 0 && (
        <details className="pick" open={data.tasks.length === 0}>
          <summary>Выбрать из всех дел ({data.pick.length})</summary>
          <ul>
            {data.pick.map((p) => (
              <li key={p}>
                <button onClick={() => add(p, "add")}>{p}</button>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
