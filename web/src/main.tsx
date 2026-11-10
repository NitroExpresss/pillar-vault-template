import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router";
import { Dashboard } from "./Dashboard";
import { Layout } from "./Layout";
import { Access } from "./pages/Access";
import { Ai } from "./pages/Ai";
import { Connections } from "./pages/Connections";
import { Doc, Missing } from "./pages/Doc";
import { Edit } from "./pages/Edit";
import { List } from "./pages/List";
import { HealthPage } from "./pages/Health";
import { Inbox } from "./pages/Inbox";
import { New } from "./pages/New";
import { ToastProvider } from "./toast";
// шрифты вшиты в сборку: закрытому хранилищу незачем отмечаться у Google
import "@fontsource-variable/unbounded";
import "@fontsource-variable/golos-text";
import "@fontsource-variable/jetbrains-mono";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <ToastProvider>
        <Layout>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/doc/*" element={<Doc />} />
            <Route path="/edit/*" element={<Edit />} />
            <Route path="/folder/*" element={<List kind="folder" />} />
            <Route path="/tag/*" element={<List kind="tag" />} />
            <Route path="/search" element={<List kind="search" />} />
            <Route path="/access" element={<Access />} />
            <Route path="/new" element={<New />} />
            <Route path="/inbox" element={<Inbox />} />
            <Route path="/health" element={<HealthPage />} />
            <Route path="/connections" element={<Connections />} />
            <Route path="/ai" element={<Ai />} />
            <Route path="*" element={<Missing what="Такого экрана нет" hint="Адрес не ведёт ни к карточке, ни к разделу." />} />
          </Routes>
        </Layout>
      </ToastProvider>
    </BrowserRouter>
  </StrictMode>,
);
