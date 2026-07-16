// ANTI-PATTERN: writes shared state by POSTing the superseded shared-storage
// REST route with a hand-rolled payload, instead of the host-mediated
// SHARED_APPEND bridge. This is the "wrong payload shape" trap.
export async function vote(base: string, key: string) {
  return fetch(`${base}/api/v1/blocks/shared-storage/increment`, {
    method: 'POST',
    body: JSON.stringify({ key, delta: 1 }),
  });
}
