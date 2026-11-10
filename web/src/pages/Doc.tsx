import { useState } from "react";
import { Link, useParams } from "react-router";
import type { DocPage, DocRef } from "../api";
import { plural } from "../lib";
import { IconPin } from "../icons";
import { useAct, type Act } from "../useAct";
import { useData } from "../useData";

// Карточка: чтение на стекле, показания и связи сбоку. Разметку разбирает
// сервер (goldmark, сырой HTML экранирован) — здесь она только вставляется;
// вики-ссылки внутри ловит оболочка и открывает экраном приложения.

export const docIdFrom = (splat: string | undefined) => (splat ?? "").split("/").map(decodeURIComponent).join("/");

export function Doc() {
  const id = docIdFrom(useParams()["*"]);
  const { data, error, reload } = useData<DocPage>(`/api/doc?id=${encodeURIComponent(id)}`);
  const act = useAct(reload);
  if (error)
    return <Missing what="Такой карточки нет" hint={`«${id}» в хранилище не нашлось — её могли переименовать или перенести.`} query={id.split("/").pop()} />;
  if (!data || data.id !== id) return <div className="page"><div className="loading" aria-busy /></div>;

  return (
    <article className="page doc">
      <nav className="crumbs" aria-label="Где карточка">
        {data.crumbs.map((c) =>
          c.url === "/" ? (
            <Link key="home" to="/">Главная</Link>
          ) : (
            <a key={c.url} href={c.url}>{c.label}</a>
          ),
        )}
      </nav>
      <div className="doc-cols">
        <div className="doc-read widget">
          <header className="doc-head">
            {(data.type || data.status) && (
              <p className="doc-kind">
                {[data.type && typeName[data.type], data.status && statusName[data.status]].filter(Boolean).join(", ")}
              </p>
            )}
            <h1>{data.title}</h1>
            {data.summary && <p className="doc-summary">{data.summary}</p>}
          </header>
          <div className="md" dangerouslySetInnerHTML={{ __html: data.html }} />
        </div>
        <aside className="doc-rail">
          <Link className="primary edit-btn" to={`/edit/${data.id.split("/").map(encodeURIComponent).join("/")}`}>
            Править карточку
          </Link>
          <Link className="ghost pin-btn" to={`/ai?card=${encodeURIComponent(data.id)}`}>
            Обсудить с ИИ
          </Link>
          <button className={data.pin ? "ghost pin-btn on" : "ghost pin-btn"} aria-pressed={data.pin}
            onClick={() => act("/pin", { id: data.id, on: data.pin ? "0" : "1" }, "Не закреплено")}>
            <IconPin /> {data.pin ? "В важном — открепить" : "Закрепить в важном"}
          </button>
          <ArchiveButton id={data.id} archived={data.status === "archived"} act={act} />
          <section className="widget">
            <dl className="facts">
              <dt>Файл</dt>
              <dd>{data.path}</dd>
              <dt>Объём</dt>
              <dd>{data.words} {plural(data.words, "слово", "слова", "слов")}</dd>
              {data.created && (<><dt>Создана</dt><dd>{data.created}</dd></>)}
              {data.updated && (<><dt>Обновлена</dt><dd>{data.updated}</dd></>)}
            </dl>
            {data.tags && data.tags.length > 0 && (
              <div className="chips">
                {data.tags.map((t) => (
                  <a key={t} className="chip" href={`/tag/${encodeURIComponent(t)}`}>{t}</a>
                ))}
              </div>
            )}
          </section>
          <Refs title="Ссылается на" refs={data.links} />
          <Refs title="Ссылаются сюда" refs={data.backlinks} />
        </aside>
      </div>
    </article>
  );
}

/** В архив — вторым нажатием (карточка уходит из поиска и «Важного»); вернуть — сразу. */
function ArchiveButton({ id, archived, act }: { id: string; archived: boolean; act: Act }) {
  const [armed, setArmed] = useState(false);
  if (archived)
    return (
      <button className="ghost pin-btn" onClick={() => act("/archive", { id, on: "0" }, "Не вернулась из архива")}>
        Вернуть из архива
      </button>
    );
  return (
    <button className={armed ? "ghost pin-btn armed" : "ghost pin-btn"} onBlur={() => setArmed(false)}
      onClick={() => (armed ? act("/archive", { id, on: "1" }, "Не ушла в архив") : setArmed(true))}>
      {armed ? "Точно в архив? Из поиска уйдёт" : "В архив"}
    </button>
  );
}

function Refs({ title, refs }: { title: string; refs: DocRef[] }) {
  if (refs.length === 0) return null;
  return (
    <section className="widget refs">
      <h2>{title}</h2>
      <ul>
        {refs.map((r) => (
          <li key={r.id}>
            {/* у шаблона заголовок — заготовка «{{date}}»: понятнее путь */}
            <a href={r.url}>{r.title.includes("{{") ? r.id : r.title}</a>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Не нашлось — сказать, чего именно нет, и дать два выхода: поиск по
 *  последнему слову адреса (так обычно находится переименованная карточка)
 *  и главная. */
export function Missing({ what, hint, query }: { what: string; hint?: string; query?: string }) {
  const words = (query ?? "").replace(/[-_]+/g, " ").trim();
  return (
    <div className="page lost">
      <section className="widget">
        <h1>{what}</h1>
        {hint && <p className="quiet">{hint}</p>}
        <div className="lost-acts">
          {words && <Link className="primary" to={`/search?q=${encodeURIComponent(words)}`}>Искать «{words}»</Link>}
          <Link className="ghost" to="/">На главную</Link>
        </div>
      </section>
    </div>
  );
}

// типы и статусы из устава (SOUL.md) — по-русски, как их зовёт человек
export const typeName: Record<string, string> = {
  fact: "факт", supplement: "добавка", guide: "гайд", hypothesis: "гипотеза", concept: "концепция",
  pillar: "столп", source: "источник", note: "заметка", question: "вопрос", area: "сфера", plan: "план",
  goal: "цель", task: "дела", archive: "архив",
};
const statusName: Record<string, string> = {
  draft: "черновик", researching: "в работе", complete: "готова", archived: "в архиве", active: "живая",
};
