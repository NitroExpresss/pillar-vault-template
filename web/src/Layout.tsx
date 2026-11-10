import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Nav, NavDir, NavDoc } from "./api";
import { AppUpdate } from "./AppItems";
import { IconChat, IconExit, IconHome, IconInbox, IconKey, IconMenu, IconPlus, IconSearch } from "./icons";
import { Sky } from "./Sky";
import { followSky } from "./phase";
import { useData } from "./useData";

// Шапка и меню. Ссылки, которые строит сервер («/doc/…», «/tag/…»), —
// адреса экранов приложения; оболочка перехватывает их и открывает экран.
// Так работают и меню, и виджеты, и вики-ссылки внутри разметки карточки.

const APP_PATHS = /^\/(doc|folder|tag|edit)\/|^\/(search|access|new|inbox|health|connections)(\?|$)/;

/** Свой путь, который умеет приложение, — внутрь роутера, остальное браузеру. */
function useAppLinks() {
  const navigate = useNavigate();
  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = (e.target as Element).closest?.("a");
      if (!a || a.target === "_blank" || a.origin !== location.origin) return;
      const path = a.pathname + a.search + a.hash;
      if (!APP_PATHS.test(path)) return;
      e.preventDefault();
      navigate(path);
    };
    document.addEventListener("click", onClick);
    return () => document.removeEventListener("click", onClick);
  }, [navigate]);
}

export function Layout({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const loc = useLocation();
  useAppLinks();
  useEffect(() => {
    setOpen(false);
    document.querySelector(".main")?.scrollTo(0, 0); // новый экран — с начала
  }, [loc.pathname, loc.search]);
  useEffect(() => followSky(), []);
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("keydown", esc);
    return () => document.removeEventListener("keydown", esc);
  }, []);

  return (
    <div className={open ? "shell nav-open" : "shell"}>
      <Sky />
      <header className="top">
        <button className="icon-btn menu-btn" onClick={() => setOpen((o) => !o)} aria-label="Меню" aria-expanded={open}>
          <IconMenu />
        </button>
        <a className="wordmark" href="/" onClick={(e) => { e.preventDefault(); navigate("/"); }}>
          Pillar Vault
        </a>
        <Search className="search top-search" />
        <nav className="top-actions" aria-label="Служебное">
          <a className="icon-btn" href="/access" title="Входы и смена ключа">
            <IconKey />
          </a>
          <form method="post" action="/logout">
            <button className="icon-btn" type="submit" title="Выйти">
              <IconExit />
            </button>
          </form>
        </nav>
      </header>
      <aside className="side" aria-label="Разделы и темы">
        <Search className="search side-search" />
        <Sidebar />
      </aside>
      <div className="scrim" onClick={() => setOpen(false)} />
      <main className="main">{children}</main>
      <Tabs here={loc.pathname} menu={() => setOpen((o) => !o)} open={open} />
    </div>
  );
}

/** Нижнее меню телефона: вкладки. */
function Tabs({ here, menu, open }: { here: string; menu: () => void; open: boolean }) {
  const { data } = useData<Nav>("/api/nav");
  const navigate = useNavigate();
  const tab = (path: string, label: string, icon: ReactNode, badge = 0) => (
    <a className={here === path ? "tab current" : "tab"} href={path} aria-current={here === path ? "page" : undefined}
      onClick={(e) => { e.preventDefault(); navigate(path); }}>
      {icon}
      <span>{label}</span>
      {badge > 0 && <span className="tab-badge">{badge}</span>}
    </a>
  );
  return (
    <nav className="tabs" aria-label="Вкладки">
      {tab("/", "Главная", <IconHome />)}
      {tab("/inbox", "Входящие", <IconInbox />, data?.inbox ?? 0)}
      {tab("/new", "Новая", <IconPlus />)}
      {tab("/ai", "ИИ", <IconChat />)}
      <button type="button" className={open ? "tab current" : "tab"} onClick={menu} aria-expanded={open}>
        <IconMenu />
        <span>Разделы</span>
      </button>
    </nav>
  );
}

// на телефоне шапке тесно — там поиск живёт в меню
function Search({ className }: { className: string }) {
  const navigate = useNavigate();
  const loc = useLocation();
  const q = loc.pathname === "/search" ? new URLSearchParams(loc.search).get("q") ?? "" : "";
  function go(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const value = new FormData(e.currentTarget).get("q")?.toString().trim();
    if (value) navigate(`/search?q=${encodeURIComponent(value)}`);
  }
  return (
    <form className={className} action="/search" method="get" role="search" onSubmit={go}>
      <IconSearch />
      <input key={q} name="q" type="search" defaultValue={q} placeholder="Найти в хранилище" autoComplete="off"
        spellCheck={false} aria-label="Поиск" enterKeyHint="search" />
    </form>
  );
}

function Sidebar() {
  const { data } = useData<Nav>("/api/nav");
  const navigate = useNavigate();
  const here = useLocation().pathname;
  if (!data) return null;
  return (
    <>
      <a className={here === "/" ? "side-link current" : "side-link"} href="/" onClick={(e) => { e.preventDefault(); navigate("/"); }}>
        Главная
      </a>
      <a className={here === "/new" ? "side-link side-new current" : "side-link side-new"} href="/new">
        + Новая карточка
      </a>
      <a className={here === "/inbox" ? "side-link current" : "side-link"} href="/inbox">
        <span className="side-label">Входящие</span>
        {data.inbox > 0 && <span className="side-count inbox-count">{data.inbox}</span>}
      </a>
      <a className={here === "/health" ? "side-link current" : "side-link"} href="/health">
        <span className="side-label">Здоровье хранилища</span>
        {data.broken > 0 && <span className="side-count broken-count" title="ошибок">{data.broken}</span>}
      </a>
      <a className={here === "/connections" ? "side-link current" : "side-link"} href="/connections">
        <span className="side-label">Шлюзы</span>
      </a>
      <a className={here === "/doc/CHANGELOG" ? "side-link current" : "side-link"} href="/doc/CHANGELOG">
        <span className="side-label">Чейнджлог</span>
      </a>
      <AppUpdate />
      <h2 className="side-head">Разделы</h2>
      {data.dirs.map((d) => (
        <Dir key={d.path} d={d} />
      ))}
      {data.files.length > 0 && <h2 className="side-head">В корне</h2>}
      {data.files.map((f) => (
        <FileLink key={f.id} f={f} here={here} twin={data.files.some((o) => o !== f && o.title === f.title)} />
      ))}
      {data.tags.length > 0 && <h2 className="side-head">Темы</h2>}
      {data.tags.map((t) => (
        <a key={t.name} className={samePath(here, t.url) ? "side-link side-tag current" : "side-link side-tag"} href={t.url}>
          <span className="side-label">{t.name}</span>
          <span className="side-meter" aria-hidden>
            <span style={{ width: `${t.share}%` }} />
          </span>
          <span className="side-count">{t.count}</span>
        </a>
      ))}
    </>
  );
}

/** Карточка в меню. Одинаковые заголовки (AGENTS.md и CLAUDE.md — оба
 *  «Точка входа агента») различает имя файла. Открытая — подсвечена. */
function FileLink({ f, here, twin }: { f: NavDoc; here: string; twin: boolean }) {
  return (
    <a className={samePath(here, f.url) ? "side-link current" : "side-link"} href={f.url}>
      <span className="side-label">{f.title}</span>
      {twin && <span className="side-count">{f.id.split("/").pop()}</span>}
    </a>
  );
}

const samePath = (here: string, url: string) => {
  try {
    return decodeURIComponent(here) === decodeURIComponent(url);
  } catch {
    return here === url;
  }
};

function Dir({ d }: { d: NavDir }) {
  const here = useLocation().pathname;
  const [open, setOpen] = useState(() => here.startsWith(d.url + "/") || decodeURIComponent(here).startsWith("/doc/" + d.path + "/"));
  const empty = d.dirs.length + d.files.length === 0;
  return (
    <div className="side-dir">
      <div className="side-row">
        <button className="side-fold" onClick={() => setOpen((o) => !o)} aria-expanded={open} disabled={empty}
          aria-label={open ? `Свернуть «${d.label}»` : `Развернуть «${d.label}»`}>
          <span className={open ? "chev open" : "chev"} />
        </button>
        <a className={samePath(here, d.url) ? "side-link current" : "side-link"} href={d.url}>
          <span className="side-label">{d.label}</span>
          <span className="side-count">{d.count}</span>
        </a>
      </div>
      {open && (
        <div className="side-kids">
          {d.dirs.map((s) => (
            <Dir key={s.path} d={s} />
          ))}
          {d.files.map((f) => (
            <FileLink key={f.id} f={f} here={here} twin={d.files.some((o) => o !== f && o.title === f.title)} />
          ))}
        </div>
      )}
    </div>
  );
}
