import { useBuzzBalance } from '@civitai/blocks-react';

// A clean scaffold: buzz is read through the host bridge hook, and the only
// direct REST calls are to unbridged catalog / dev-token routes.
export function App() {
  const { balance } = useBuzzBalance();
  return balance;
}

async function loadCatalog() {
  // Legit: public catalog read, no host bridge.
  const res = await fetch('/api/v1/blocks/models?limit=20');
  return res.json();
}
