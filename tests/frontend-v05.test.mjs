import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { createRefresh } from "../web/src/refresh.js";

const deferred = () => {
  let resolve;
  const promise = new Promise((r) => (resolve = r));
  return { promise, resolve };
};

test("filter changes abort old work and discard an out-of-order response", async () => {
  const requests = [],
    committed = [],
    errors = [];
  const refresh = createRefresh(
    (signal) => {
      const request = { ...deferred(), signal };
      requests.push(request);
      return request.promise; // Deliberately ignores abort, as a late network response might.
    },
    (value) => committed.push(value),
    (error) => errors.push(error),
  );
  const first = refresh();
  assert.equal(refresh(), first, "polling and notifications should coalesce");
  await Promise.resolve();
  const second = refresh(true);
  await Promise.resolve();
  assert.ok(requests[0].signal.aborted);
  requests[1].resolve("new filter");
  await second;
  requests[0].resolve("old filter");
  await first;
  assert.deepEqual(committed, ["new filter"]);
  assert.deepEqual(errors, []);
});

test("unmount aborts requests and prevents late state/error updates", async () => {
  const request = deferred();
  let signal,
    commits = 0,
    errors = 0,
    settled = 0;
  const refresh = createRefresh(
    (s) => {
      signal = s;
      return request.promise;
    },
    () => commits++,
    () => errors++,
    () => settled++,
  );
  const pending = refresh();
  await Promise.resolve();
  refresh.stop();
  assert.ok(signal.aborted);
  request.resolve("too late");
  await pending;
  await refresh();
  assert.deepEqual([commits, errors, settled], [0, 0, 0]);
});

test("failed work releases the slot and allows retry", async () => {
  let calls = 0,
    errors = 0,
    applied;
  const refresh = createRefresh(
    () => {
      if (++calls === 1) throw new Error("network unavailable");
      return "recovered";
    },
    (value) => (applied = value),
    () => errors++,
  );
  await refresh();
  await refresh();
  assert.equal(errors, 1);
  assert.equal(applied, "recovered");
});

test("late socket callbacks cannot revive an unloaded page or private connection", () => {
  const sockets = [],
    timers = new Map();
  let timerID = 0;
  const context = {
    ref: (value) => ({ value }),
    location: { protocol: "https:", host: "monitor.example" },
    WebSocket: class {
      constructor(url) {
        this.url = url;
        sockets.push(this);
      }
      close() {
        this.closed = true;
        this.onclose?.();
      }
    },
    setTimeout: (callback, delay) => {
      timers.set(++timerID, { callback, delay });
      return timerID;
    },
    clearTimeout: (id) => timers.delete(id),
    Math,
  };
  vm.createContext(context);
  const source = fs
    .readFileSync(new URL("../web/src/state.js", import.meta.url), "utf8")
    .replace(/^import .*;\r?\n/, "")
    .replaceAll("export ", "");
  vm.runInContext(
    source + "\nglobalThis.fixture = {startLive,stopLive,live,revision};",
    context,
  );
  const { startLive, stopLive, live, revision } = context.fixture;
  startLive(false);
  const old = sockets[0];
  old.onopen();
  old.onclose();
  assert.equal(timers.size, 1);
  stopLive();
  old.onmessage();
  old.onclose();
  assert.equal(timers.size, 0);
  assert.equal(live.value, false);
  const before = revision.value;
  startLive(true);
  old.onopen();
  assert.equal(revision.value, before);
  assert.equal(sockets[1].url, "wss://monitor.example/api/public/ws");
  stopLive();
});
