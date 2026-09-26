// CONTROL FIXTURE C — "asks".
//
// A genpost-shaped app derived from the page-money scaffold, keeping the
// scaffold's own consent discipline: Generate requests the budgeted scope when
// the token does not already carry it.
//
// Its twin, App.blind.tsx, is identical but for the `if (!granted) { ... }`
// branch below and the `void` bindings that keep the SDK surface matched across
// the two bundles. The pair is the point: any difference in the unconsented
// arm's verdict is attributable to that branch and to nothing else.
//
// ⚠ NOT "byte-identical" — an earlier header claimed that, and a reader who
// diffed the pair found five hunks with no way to tell which were load-bearing.
import { useCallback, useState } from 'react';

import {
  useBlockContext,
  useBlockToken,
  useBuzzWorkflow,
  useRequestConsent,
} from '@civitai/blocks-react';

const BUDGETED_SCOPE = 'ai:write:budgeted';

function hasBudgetedScope(scopes: readonly string[] | undefined): boolean {
  return Array.isArray(scopes) && scopes.includes(BUDGETED_SCOPE);
}

export function App() {
  const { ready } = useBlockContext();
  const token = useBlockToken();
  const { estimate, submit } = useBuzzWorkflow();
  const { requestConsent } = useRequestConsent();

  const [prompt, setPrompt] = useState('');
  const [status, setStatus] = useState<'ready' | 'generating'>('ready');
  const [generated, setGenerated] = useState(false);

  const granted = hasBudgetedScope(token.scopes);

  const runGeneration = useCallback(async () => {
    setStatus('generating');
    try {
      await estimate({ prompt } as never);
      await submit({ prompt } as never);
      setGenerated(true);
    } catch {
      /* the inline transport rejects; the status machine is what is graded */
    } finally {
      setStatus('ready');
    }
  }, [estimate, submit, prompt]);

  const onGenerate = useCallback(() => {
    // THE ONE BRANCH THAT SEPARATES THIS FIXTURE FROM ITS TWIN.
    if (!granted) {
      requestConsent({ scopes: [BUDGETED_SCOPE] });
      return;
    }
    void runGeneration();
  }, [granted, requestConsent, runGeneration]);

  if (!ready) return <div data-testid="status">loading</div>;

  return (
    <div>
      <input
        data-testid="prompt"
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
      />
      <div data-testid="status">{status}</div>
      <button disabled={prompt.trim().length === 0} onClick={onGenerate}>
        Generate
      </button>
      <button disabled={!generated}>Post</button>
    </div>
  );
}
