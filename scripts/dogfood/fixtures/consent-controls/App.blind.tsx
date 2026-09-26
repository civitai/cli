// CONTROL FIXTURE B — "blind". THE UNCONSENTED ARM'S NEGATIVE CONTROL.
//
// A genpost-shaped app derived from the page-money scaffold that reaches the
// Generate click and spends WITHOUT ever asking the host for consent — the exact
// defect class `ab-img-poster v0.1.0` shipped with, and the one the unconsented
// arm was built to catch.
//
// Its twin, App.asks.tsx, is BYTE-IDENTICAL except that it keeps the
// `if (!granted) { ... requestConsent(...) }` branch. The pair is the point: any
// difference in the unconsented arm's verdict is attributable to that branch and
// to nothing else.
import { useCallback, useRef, useState } from 'react';

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
  // Imported and bound exactly as the twin does, so the SDK surface loaded into
  // the bundle is the same on both sides and the only delta is the CALL.
  const { requestConsent } = useRequestConsent();
  void requestConsent;

  const [prompt, setPrompt] = useState('');
  const [status, setStatus] = useState<'ready' | 'generating'>('ready');
  const [generated, setGenerated] = useState(false);
  const consentPendingRef = useRef(false);
  void consentPendingRef;

  const granted = hasBudgetedScope(token.scopes);
  void granted;

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
    // THE MISSING BRANCH. No consent is requested; generation is attempted on
    // whatever the token happens to carry.
    void runGeneration();
  }, [runGeneration]);

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
