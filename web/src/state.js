import { ref } from "vue";
export const user = ref(null);
export const live = ref(false);
export const revision = ref(0);
let socket,
  timer,
  generation = 0,
  mode = null;
export function startLive(publicView = false) {
  const next = publicView ? "/api/public/ws" : "/api/ws";
  if (mode === next) return;
  stopLive();
  mode = next;
  const current = generation;
  let retries = 0;
  function connect() {
    if (generation !== current) return;
    const connection = new WebSocket(
      (location.protocol === "https:" ? "wss:" : "ws:") +
        "//" +
        location.host +
        next,
    );
    socket = connection;
    connection.onopen = () => {
      if (generation !== current) return;
      live.value = true;
      retries = 0;
      revision.value++;
    };
    connection.onmessage = () => {
      if (generation === current) revision.value++;
    };
    connection.onclose = () => {
      if (generation !== current) return;
      live.value = false;
      const delay = Math.min(30000, 1000 * 2 ** Math.min(retries++, 5));
      timer = setTimeout(connect, delay + Math.floor(Math.random() * 1000));
    };
    connection.onerror = () => connection.close();
  }
  connect();
}
export function stopLive() {
  generation++;
  mode = null;
  clearTimeout(timer);
  socket?.close();
  socket = null;
  live.value = false;
}
