"use strict";

// Preloaded (`node --require`) into the wrapper under test: replaces https.get
// so no test ever reaches the network, and records every attempt.
//
//   CIVITAI_TEST_HTTP_LOG   file that gets one requested URL per line
//   CIVITAI_TEST_HTTP_DIR   optional; a file named like the URL's last path
//                           segment is served as a 200. Anything else — and
//                           everything when this is unset — fails like a host
//                           that cannot be reached.

const fs = require("fs");
const path = require("path");
const https = require("https");
const { EventEmitter } = require("events");

https.get = function fakeGet(url, _opts, onResponse) {
  fs.appendFileSync(process.env.CIVITAI_TEST_HTTP_LOG, `${url}\n`);
  const req = new EventEmitter();
  req.setTimeout = () => req;
  req.destroy = () => {};

  const dir = process.env.CIVITAI_TEST_HTTP_DIR;
  const file = dir && path.join(dir, path.basename(new URL(url).pathname));
  setImmediate(() => {
    if (!file || !fs.existsSync(file)) {
      req.emit("error", new Error("connect ENETUNREACH (fake-https)"));
      return;
    }
    const res = new EventEmitter();
    res.statusCode = 200;
    res.headers = {};
    res.resume = () => {};
    onResponse(res);
    res.emit("data", fs.readFileSync(file));
    res.emit("end");
  });
  return req;
};
