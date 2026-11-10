import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";

// Ответ сервера после записи: «такого дела нет в списке», «нет связи».
// Успех молчит — его и так видно по самому блоку.

interface Toast {
  id: number;
  text: string;
}

const Ctx = createContext<(text: string) => void>(() => {});

export function ToastProvider({ children }: { children: ReactNode }) {
  const [list, setList] = useState<Toast[]>([]);
  const seq = useRef(0);
  const say = useCallback((text: string) => {
    const id = ++seq.current;
    setList((l) => [...l, { id, text }]);
    setTimeout(() => setList((l) => l.filter((t) => t.id !== id)), 4200);
  }, []);
  return (
    <Ctx.Provider value={say}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {list.map((t) => (
          <div key={t.id} className="toast">
            {t.text}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}

export const useToast = () => useContext(Ctx);
