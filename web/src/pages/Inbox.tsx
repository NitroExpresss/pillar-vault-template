import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { postJSON, type DocRef } from "../api";
import { plural } from "../lib";
import { useAct } from "../useAct";
import { useData } from "../useData";

// Входящие: мысли из «Захвата» и черновики карточек. Руками — в дела,
// в карточку (форма; после неё мысль уходит отсюда сама) или удалить
// (вторым нажатием). Кнопка «ИИ» — вердикт модели через шлюз по умолчанию
// (или тот, что выбран в чате): дело, карточка или мусор — применить
// одним нажатием, модель только советует.

interface InboxItem {
  section: string;
  text: string;
  clean: string;
  when?: string;
  link?: boolean;
}

interface InboxData {
  items: InboxItem[];
  files: DocRef[];
  src: string;
}

/** Заголовок будущей карточки — начало мысли, по словам. */
const titleOf = (s: string) => {
  if (s.length <= 60) return s;
  const cut = s.slice(0, 60);
  return cut.slice(0, cut.lastIndexOf(" ") > 20 ? cut.lastIndexOf(" ") : 60) + "…";
};

interface AiVerdict {
  ok: boolean;
  action?: "task" | "card" | "drop";
  title?: string;
  why?: string;
  message?: string;
}

/** Вердикт ИИ по мысли: что предложил и кнопка «применить». */
function AiSuggest({ verdict, onApply, onClose }:
  { verdict: AiVerdict; onApply: () => void; onClose: () => void }) {
  if (!verdict.ok) return <p className="inbox-ai fail">{verdict.message || "ИИ не ответил"}</p>;
  const label = verdict.action === "task" ? "В дела" : verdict.action === "drop" ? "Мусор" : "В карточку";
  return (
    <p className="inbox-ai">
      <span className="inbox-ai-tag">ИИ: {label}</span> {verdict.title}
      {verdict.why && <span className="inbox-ai-why"> — {verdict.why}</span>}
      <button className="ghost" onClick={onApply}>Применить</button>
      <button className="ghost" onClick={onClose}>Скрыть</button>
    </p>
  );
}

export function Inbox() {
  const { data, reload } = useData<InboxData>("/api/inbox");
  const act = useAct(reload);
  const navigate = useNavigate();
  const [armed, setArmed] = useState<string | null>(null);
  const [ai, setAi] = useState<Record<string, AiVerdict | "busy">>({});

  /** Вердикт модели по мысли. Шлюз — выбранный в чате, иначе первый. */
  async function askAi(text: string) {
    setAi((m) => ({ ...m, [text]: "busy" }));
    const res = await postJSON<AiVerdict>("/api/ai/inbox", { conn: localStorage.getItem("pv.ai.conn") ?? "", text });
    setAi((m) => ({ ...m, [text]: res }));
  }

  /** Применить вердикт: дело — в дела, карточка — форма новой, мусор — удалить. */
  function applyAi(text: string, v: AiVerdict) {
    setAi((m) => {
      const n = { ...m };
      delete n[text];
      return n;
    });
    if (v.action === "task") {
      void act("/inbox", { do: "task", text }, "Дело не записано");
    } else if (v.action === "drop") {
      void act("/inbox", { do: "remove", text }, "Не удалено");
    } else {
      navigate(`/new?${new URLSearchParams({ title: v.title ?? "", text, inbox: text })}`);
    }
  }

  if (!data) return <div className="page"><div className="loading" aria-busy /></div>;
  const n = data.items.length;
  const total = n + data.files.length;
  const sections = [...new Set(data.items.map((i) => i.section))];

  return (
    <div className="page inbox">
      <header className="list-head">
        <h1>Входящие</h1>
        <p>{total === 0 ? "Всё разобрано" : `${total} ${plural(total, "запись ждёт", "записи ждут", "записей ждут")} разбора`}</p>
      </header>
      {n === 0 ? (
        <section className="widget empty">
          <p>Мысли из виджета «Захват» появятся здесь. Разобранная мысль становится делом или карточкой — или уходит.</p>
          <Link className="ghost" to="/">На главную</Link>
        </section>
      ) : (
        sections.map((s) => (
          <section key={s} className="widget inbox-sec">
            <h2>{s}</h2>
            <ul>
              {data.items.filter((i) => i.section === s).map((i) => (
                <li key={i.text} className="inbox-item">
                  <div className="inbox-text">
                    {i.link ? <a href={i.clean} target="_blank" rel="noreferrer noopener">{i.clean}</a> : i.clean}
                    {i.when && <span className="inbox-when">{i.when}</span>}
                  </div>
                  {ai[i.text] && ai[i.text] !== "busy" && (
                    <AiSuggest
                      verdict={ai[i.text] as AiVerdict}
                      onApply={() => applyAi(i.text, ai[i.text] as AiVerdict)}
                      onClose={() =>
                        setAi((m) => {
                          const n = { ...m };
                          delete n[i.text];
                          return n;
                        })
                      }
                    />
                  )}
                  <div className="inbox-acts">
                    <button className="ghost" disabled={ai[i.text] === "busy"} onClick={() => void askAi(i.text)}>
                      {ai[i.text] === "busy" ? "ИИ думает…" : "ИИ"}
                    </button>
                    <button className="ghost" onClick={() => act("/inbox", { do: "task", text: i.text }, "Дело не записано")}>
                      В дела
                    </button>
                    <Link className="ghost"
                      to={`/new?${new URLSearchParams({ title: i.link ? "" : titleOf(i.clean), text: i.clean, inbox: i.text })}`}>
                      В карточку
                    </Link>
                    <button className={armed === i.text ? "ghost armed" : "ghost"}
                      onClick={() => {
                        if (armed !== i.text) return setArmed(i.text);
                        setArmed(null);
                        act("/inbox", { do: "remove", text: i.text }, "Не удалено");
                      }}>
                      {armed === i.text ? "Точно удалить?" : "Удалить"}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </section>
        ))
      )}
      {data.files.length > 0 && (
        <section className="widget inbox-sec">
          <h2>Карточки во входящих</h2>
          <ul>
            {data.files.map((d) => (
              <li key={d.id} className="inbox-item">
                <div className="inbox-text">
                  <a href={d.url}>{d.title}</a>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
