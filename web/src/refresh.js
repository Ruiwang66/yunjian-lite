// Coalesce notifications while a request is running. A changed filter replaces
// it; late responses and unmounted pages can never commit stale state.
export function createRefresh(load, apply, onError, onSettled = () => {}) {
  let active,
    controller,
    generation = 0,
    stopped = false;
  function refresh(replace = false) {
    if (stopped) return Promise.resolve();
    if (active && replace !== true) return active;
    const current = ++generation;
    controller?.abort();
    controller = new AbortController();
    const signal = controller.signal;
    active = (async () => {
      try {
        const value = await Promise.resolve().then(() => load(signal));
        if (!stopped && current === generation) apply(value);
      } catch (error) {
        if (!stopped && current === generation && error.name !== "AbortError")
          onError(error);
      } finally {
        if (!stopped && current === generation) {
          active = null;
          onSettled();
        }
      }
    })();
    return active;
  }
  refresh.stop = () => {
    stopped = true;
    generation++;
    controller?.abort();
    active = null;
  };
  return refresh;
}
