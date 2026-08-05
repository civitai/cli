// FIXTURE — deliberately INERT. Never shipped to an author.
//
// The runtime guard's second negative control: a file that loads cleanly and
// does nothing. It proves the driver can distinguish "the emitter ran and said
// nothing" from "the emitter ran and acked" — if this one is reported as a
// successful handshake, the driver is wired to nothing and every green it
// produces is meaningless.
(function () {
  'use strict';
  var unused = 1;
  return unused;
})();
