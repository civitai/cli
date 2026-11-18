// NOT an anti-pattern — this fixture must scan CLEAN.
//
// `GET /api/v1/blocks/buzz` is a live route returning the caller's own
// `{ blue, green, yellow }` balance. A `buzz-rest` rule used to flag this file
// as a fetch of a removed route; it was removed, and this fixture is the guard
// against it coming back.
export async function getBuzz(base: string, token: string) {
  const res = await fetch(`${base}/api/v1/blocks/buzz`, {
    headers: { authorization: `Bearer ${token}` },
  });
  return res.json();
}
