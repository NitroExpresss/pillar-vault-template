import { useEffect, useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router";
import { send } from "../api";
import { useToast } from "../toast";
import { useData } from "../useData";

// Новая карточка. По умолчанию — черновик во входящие: заголовок и текст,
// остальное дооформляется при разборе входящих. Сразу в раздел — полная
// форма по уставу: summary, тип, раздел, тема из дерева, теги.
// Frontmatter собирает сервер (/api/new).

interface NewForm {
  folders: { path: string; label: string }[];
  types: { value: string; label: string; folder?: string }[];
  taxonomy: string[];
}

export function New() {
  const { data } = useData<NewForm>("/api/new");
  const q = new URLSearchParams(useLocation().search);
  const navigate = useNavigate();
  const say = useToast();
  const [type, setType] = useState(q.get("type") ?? "note");
  const [folder, setFolder] = useState(q.get("folder") ?? "");
  const [folderTouched, setFolderTouched] = useState(!!q.get("folder"));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // из мысли «В карточку» или с выбранным разделом — сразу полная форма
  const [toInbox, setToInbox] = useState(!q.get("folder") && !q.get("inbox"));

  // раздел по типу: факт — в «Факты», цель — в «Цели»; выбрал сам — не трогаем
  useEffect(() => {
    if (!data || folderTouched) return;
    const def = data.types.find((t) => t.value === type)?.folder;
    if (def && data.folders.some((f) => f.path === def)) setFolder(def);
    else if (!folder) setFolder(data.folders[0]?.path ?? "");
  }, [data, type, folderTouched, folder]);

  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    let res: { ok?: boolean; url?: string; message?: string };
    try {
      const r = await fetch("/api/new", {
        method: "POST",
        headers: { "X-Pillar-Api": "1" },
        credentials: "same-origin",
        body: new URLSearchParams([...f.entries()].map(([k, v]) => [k, String(v)])),
      });
      if (r.status === 401) {
        window.location.assign("/?next=" + encodeURIComponent(location.pathname + location.search));
        return;
      }
      res = await r.json();
    } catch {
      res = { message: "Нет связи с сервером — форма осталась, нажмите ещё раз" };
    }
    setBusy(false);
    if (!res.ok || !res.url) {
      setError(res.message || "Карточка не создана");
      return;
    }
    const from = q.get("inbox");
    if (from) {
      const done = await send("/inbox", { do: "remove", text: from });
      if (!done.ok) say("Карточка создана, но мысль осталась во входящих: " + (done.message || "не убрана"));
      else say("Карточка создана, мысль разобрана");
    } else if (toInbox) {
      say("Во входящих — разберите, когда будет удобно");
      navigate("/inbox");
      return;
    } else say("Карточка создана");
    navigate(res.url);
  }

  if (!data) return <div className="page"><div className="loading" aria-busy /></div>;
  // тема из адреса: из дерева — в «Тему», свободный тег — в «Ещё теги»
  const asked = q.get("tag");
  const tax = asked && data.taxonomy.includes(asked) ? asked : undefined;
  return (
    <div className="page editor">
      <header className="list-head">
        <h1>Новая карточка</h1>
        <p>{toInbox ? "Черновик во входящие: заголовок и текст, остальное — при разборе" : "Сразу в раздел по уставу: тип, summary и тема обязательны"}</p>
      </header>
      <form className="widget new-form" onSubmit={create} autoComplete="off">
        <div className="new-mode wide" role="radiogroup" aria-label="Куда">
          <button type="button" role="radio" aria-checked={toInbox} className={toInbox ? "on" : undefined} onClick={() => setToInbox(true)}>
            Во входящие — разобрать позже
          </button>
          <button type="button" role="radio" aria-checked={!toInbox} className={!toInbox ? "on" : undefined} onClick={() => setToInbox(false)}>
            Сразу в раздел
          </button>
        </div>
        {toInbox && <input type="hidden" name="to" value="inbox" />}
        <label className="wide">
          Заголовок
          <input name="title" required maxLength={120} autoFocus defaultValue={q.get("title") ?? ""} />
        </label>
        {!toInbox && (
        <>
        <label className="wide">
          Summary — одна строка: о чём карточка
          <input name="summary" required maxLength={300} defaultValue={q.get("summary") ?? ""}
            placeholder="По ней карточку находят, не открывая" />
        </label>
        <label>
          Тип
          <select name="type" value={type} onChange={(e) => setType(e.target.value)}>
            {data.types.map((t) => (
              <option key={t.value} value={t.value}>{t.label}</option>
            ))}
          </select>
        </label>
        <label>
          Раздел
          <select name="folder" value={folder} onChange={(e) => { setFolder(e.target.value); setFolderTouched(true); }}>
            {data.folders.map((f) => (
              <option key={f.path} value={f.path}>{f.label}</option>
            ))}
          </select>
        </label>
        <label>
          Тема — из дерева
          <select name="tag" defaultValue={tax ?? data.taxonomy.find((t) => t === "pkm") ?? data.taxonomy[0]}>
            {data.taxonomy.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </label>
        <label>
          Ещё теги — через запятую
          <input name="tags" placeholder="сон, режим-дня" defaultValue={tax ? "" : (q.get("tag") ?? "")} />
        </label>
        {type === "goal" && (
          <label>
            Срок
            <input name="deadline" type="date" />
          </label>
        )}
        </>
        )}
        <label className="wide">
          {toInbox ? "Текст — мысль, ссылка, что известно" : "Текст — можно потом"}
          <textarea name="body" rows={6} defaultValue={q.get("text") ?? ""} />
        </label>
        {error && <p className="new-error wide" role="alert">{error}</p>}
        <div className="form-bar wide">
          <button className="primary" type="submit" disabled={busy}>{busy ? "Создаю" : toInbox ? "Во входящие" : "Создать карточку"}</button>
          <button className="ghost" type="button" onClick={() => navigate(-1)}>Отмена</button>
        </div>
      </form>
    </div>
  );
}
