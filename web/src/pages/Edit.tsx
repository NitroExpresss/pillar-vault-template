import { useEffect, useState, type KeyboardEvent } from "react";
import { Link, useNavigate, useParams } from "react-router";
import type { CardRaw, CardSaved } from "../api";
import { getJSON } from "../api";
import { useToast } from "../toast";
import { docIdFrom, Missing } from "./Doc";

// Правка карточки целиком: markdown как есть. Если карточку успели поправить
// (агент, бот, другая вкладка), сервер не пишет вслепую — показываем, что
// лежит на диске, и повторное сохранение пишет осознанно.

export function Edit() {
  const id = docIdFrom(useParams()["*"]);
  const navigate = useNavigate();
  const say = useToast();
  const [card, setCard] = useState<CardRaw | null>(null);
  const [text, setText] = useState("");
  const [ver, setVer] = useState("");
  const [disk, setDisk] = useState("");
  const [saving, setSaving] = useState(false);
  const [missing, setMissing] = useState(false);
  const back = `/doc/${id.split("/").map(encodeURIComponent).join("/")}`;

  useEffect(() => {
    getJSON<CardRaw>(`/api/card?id=${encodeURIComponent(id)}`)
      .then((c) => {
        setCard(c);
        setText(c.raw);
        setVer(c.ver);
      })
      .catch(() => setMissing(true));
  }, [id]);

  async function save() {
    if (saving) return;
    setSaving(true);
    let res: CardSaved;
    try {
      const r = await fetch("/api/card", {
        method: "POST",
        headers: { "X-Pillar-Api": "1" },
        credentials: "same-origin",
        body: new URLSearchParams({ id, content: text, ver }),
      });
      if (r.status === 401) {
        window.location.assign("/");
        return;
      }
      res = (await r.json()) as CardSaved;
    } catch {
      res = { ok: false, message: "Нет связи с сервером — текст остался здесь, сохраните ещё раз" };
    }
    setSaving(false);
    if (res.ok) {
      navigate(back);
      return;
    }
    if (res.disk !== undefined) {
      setDisk(res.disk);
      setVer(res.ver ?? "");
    }
    say(res.message || "Не сохранено");
  }

  function keys(e: KeyboardEvent<HTMLTextAreaElement>) {
    if ((e.ctrlKey || e.metaKey) && e.key === "s") {
      e.preventDefault();
      void save();
    } else if (e.key === "Tab") {
      e.preventDefault();
      const el = e.currentTarget;
      el.setRangeText("  ", el.selectionStart, el.selectionEnd, "end");
      setText(el.value);
    }
  }

  if (missing)
    return <Missing what="Такой карточки нет" hint={`Править нечего: «${id}» в хранилище не нашлось.`} query={id.split("/").pop()} />;
  if (!card) return <div className="page"><div className="loading" aria-busy /></div>;
  return (
    <div className="page editor">
      <header className="list-head">
        <h1>{card.title}</h1>
        <p>Правка, {card.path}</p>
      </header>
      {disk && (
        <details className="widget disk" open>
          <summary>Пока вы правили, карточка изменилась. Так она выглядит сейчас</summary>
          <pre>{disk}</pre>
        </details>
      )}
      <textarea className="editor-text widget" value={text} onChange={(e) => setText(e.target.value)} onKeyDown={keys}
        spellCheck={false} aria-label={`Текст карточки «${card.title}»`} />
      <div className="editor-bar">
        <button className="primary" onClick={save} disabled={saving}>
          {saving ? "Сохраняю" : disk ? "Сохранить мою версию" : "Сохранить"}
        </button>
        <Link className="ghost" to={back}>Отмена</Link>
        <span className="quiet hint">Ctrl+S — сохранить</span>
      </div>
    </div>
  );
}
