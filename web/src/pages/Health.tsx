import { useState } from "react";
import { Link } from "react-router";
import { plural } from "../lib";
import { useData } from "../useData";

// Здоровье хранилища: тот же линтер, что в CI. Без агента чинит сам
// хозяин — поэтому у каждой ошибки ссылка прямо в правку карточки.

interface Issue {
  file: string;
  id?: string;
  title?: string;
  what: string;
}

interface Health {
  cards: number;
  errors: Issue[];
  warnings: Issue[];
}

const editHref = (id: string) => `/edit/${id.split("/").map(encodeURIComponent).join("/")}`;

function Issues({ list }: { list: Issue[] }) {
  // по карточкам: у одной карточки бывает несколько бед сразу
  const byFile = new Map<string, Issue[]>();
  for (const i of list) byFile.set(i.file, [...(byFile.get(i.file) ?? []), i]);
  return (
    <ul className="health-list">
      {[...byFile].map(([file, items]) => (
        <li key={file}>
          <div className="health-card">
            <b>{items[0].title ?? file}</b>
            <span className="health-file">{file}</span>
          </div>
          <ul>
            {items.map((i) => (
              <li key={i.what}>{i.what}</li>
            ))}
          </ul>
          {items[0].id && (
            <div className="health-acts">
              <Link className="ghost" to={editHref(items[0].id)}>Править</Link>
              <a className="ghost" href={`/doc/${items[0].id.split("/").map(encodeURIComponent).join("/")}`}>Открыть</a>
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}

export function HealthPage() {
  const { data } = useData<Health>("/api/health");
  const [showWarn, setShowWarn] = useState(false);
  if (!data) return <div className="page"><div className="loading" aria-busy /></div>;
  const e = data.errors.length, w = data.warnings.length;
  return (
    <div className="page health">
      <header className="list-head">
        <h1>Здоровье хранилища</h1>
        <p>
          {data.cards} {plural(data.cards, "карточка", "карточки", "карточек")}:{" "}
          {e === 0 ? "ошибок нет" : `${e} ${plural(e, "ошибка", "ошибки", "ошибок")}`}
          {w > 0 && `, ${w} ${plural(w, "предупреждение", "предупреждения", "предупреждений")}`}
        </p>
      </header>
      {e === 0 ? (
        <section className="widget empty">
          <p>Всё по уставу: ссылки живые, у каждой карточки summary и тема из дерева.</p>
        </section>
      ) : (
        <section className="widget">
          <h2 className="health-h">Ошибки — чинить</h2>
          <p className="quiet">Битая ссылка, нет summary, тема не из дерева. С ними карточка теряется в поиске, а выкладка останавливается.</p>
          <Issues list={data.errors} />
        </section>
      )}
      {w > 0 && (
        <section className="widget">
          <button className="ghost" onClick={() => setShowWarn((s) => !s)} aria-expanded={showWarn}>
            {showWarn ? "Скрыть предупреждения" : `Предупреждения: ${w}`}
          </button>
          {showWarn && <Issues list={data.warnings} />}
        </section>
      )}
    </div>
  );
}
