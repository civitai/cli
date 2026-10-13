import { useBuzzBalance } from '@civitai/blocks-react';

// A clean scaffold: buzz is read through the bridge hook (still supported), and
// the catalog through a live REST route.
export function App() {
  const { balance } = useBuzzBalance();
  return balance;
}

async function loadCatalog() {
  // Legit: public catalog read.
  const res = await fetch('/api/v1/blocks/models?limit=20');
  return res.json();
}
