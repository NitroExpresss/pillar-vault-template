import type { ReactNode } from "react";
import { Link, useLocation, useParams } from "react-router";
import type { DocRef, ListPage, Nav } from "../api";
import { plural } from "../lib";
import { useData } from "../useData";
import { docIdFrom, Missing, typeName } from "./Doc";

// Раздел, тема и поиск — один экран: список карточек строками, а не сеткой
// одинаковых плиток. У строки одна работа — дать понять, о чём карточка.

/** Тип, раздел и первые темы — без повторов: тип «план» и тег «план» — одно. */
function meta(d: DocRef) {
  const parts = [d.type && typeName[d.type], d.folder, ...(d.tags ?? []).slice(0, 3)].filter(Boolean) as string[];
  return [...new Set(parts.map((p) => p.toLowerCase()))].join(", ");
}

/** Найденное слово подсвечено — видно, за что карточка попала в выдачу. */
function Marked({ text, q }: { text: string; q: string }) {
  const needle = q.trim().toLowerCase();
  if (!needle) return <>{text}</>;
  const low = text.toLowerCase();
  const out: ReactNode[] = [];
  let at = 0;
  for (let i = low.indexOf(needle); i >= 0; i = low.indexOf(needle, at)) {
    out.push(text.slice(at, i), <mark key={i}>{text.slice(i, i + needle.length)}</mark>);
    at = i + needle.length;
  }
  out.push(text.slice(at));
  return <>{out}</>;
}

const cards = (n: number) => `${n} ${plural(n, "карточка", "карточки", "карточек")}`;

export function List({ kind }: { kind: "folder" | "tag" | "search" }) {
  const splat = docIdFrom(useParams()["*"]);
  const q = new URLSearchParams(useLocation().search).get("q") ?? "";
  const param = kind === "search" ? `q=${encodeURIComponent(q)}` : `${kind}=${encodeURIComponent(splat)}`;
  const { data, error } = useData<ListPage>(`/api/list?${param}`);
  if (error)
    return <Missing what={kind === "tag" ? "Такой темы нет" : "Такого раздела нет"} hint="Её могли переименовать — поищите по слову." query={splat.split("/").pop()} />;
  if (!data) return <div className="page"><div className="loading" aria-busy /></div>;

  const n = data.items.length;
  const mark = kind === "search" ? q : "";
  const heading = kind === "search" ? (q ? `«${q}»` : "Поиск") : data.title;
  const sub =
    kind === "search"
      ? q
        ? n === 0 ? "Ничего не нашлось" : `Нашлось: ${cards(n)}`
        : "Введите слово в строку поиска"
      : `${kind === "tag" ? "Тема" : "Раздел"}, ${cards(n)}`;

  return (
    <div className="page listing">
      <header className="list-head">
        <h1>{heading}</h1>
        <p>{sub}</p>
        {kind === "folder" && (
          <Link className="ghost list-new" to={`/new?folder=${encodeURIComponent(splat)}`}>+ Новая карточка здесь</Link>
        )}
        {kind === "tag" && data.title && (
          <Link className="ghost list-new" to={`/new?tag=${encodeURIComponent(data.title)}`}>+ Новая карточка с этой темой</Link>
        )}
      </header>
      {data.kids && data.kids.length > 0 && (
        <div className="chips list-kids">
          {data.kids.map((k) => (
            <a key={k.name} className="chip" href={k.url}>
              {k.name.slice(data.title.length + 1)} <b>{k.count}</b>
            </a>
          ))}
        </div>
      )}
      {n === 0 ? (
        <Empty searching={kind === "search" && q !== ""} />
      ) : (
        <ul className="doc-list widget">
          {data.items.map((d) => (
            <li key={d.id}>
              <a href={d.url}>
                <span className="doc-list-title"><Marked text={d.title} q={mark} /></span>
                {d.snippet && d.snippet !== d.title && (
                  <span className="doc-list-text"><Marked text={d.snippet} q={mark} /></span>
                )}
                <span className="doc-list-meta">{meta(d)}</span>
              </a>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** Пусто — значит, подсказать, куда идти: поиск ищет и начало слова, а темы
 *  хранилища — готовые входы. */
function Empty({ searching }: { searching: boolean }) {
  const { data } = useData<Nav>("/api/nav");
  return (
    <section className="widget empty">
      <p>
        {searching
          ? "Поиск смотрит заголовки, summary, теги и текст. Попробуйте начало слова или зайдите через тему:"
          : "Здесь пока нет карточек. Темы хранилища:"}
      </p>
      {data && (
        <div className="chips">
          {data.tags.slice(0, 8).map((t) => (
            <a key={t.name} className="chip" href={t.url}>
              {t.name} <b>{t.count}</b>
            </a>
          ))}
        </div>
      )}
      <Link className="ghost" to="/">На главную</Link>
    </section>
  );
}
