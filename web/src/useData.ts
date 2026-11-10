import { useCallback, useEffect, useState } from "react";
import { getJSON } from "./api";

/**
 * Данные экрана с сервера. set — оптимистичная правка до ответа, reload —
 * сверка с сервером после записи: источник истины — карточки, а не браузер.
 */
export function useData<T>(url: string) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      setData(await getJSON<T>(url));
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [url]);

  useEffect(() => {
    void reload();
    // вернулся в приложение — данные могли поменять агент или виджет
    const onShow = () => document.visibilityState === "visible" && void reload();
    document.addEventListener("visibilitychange", onShow);
    return () => document.removeEventListener("visibilitychange", onShow);
  }, [reload]);

  return { data, error, set: setData, reload };
}
