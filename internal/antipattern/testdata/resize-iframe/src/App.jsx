import { useEffect, useState } from 'react';

// A page app that sizes itself to content. This is the shape the `page-vite`
// template shipped before #206: the host renders a page block full-viewport and
// marks RESIZE_IFRAME N/A, so this effect runs forever into nothing.
export default function App() {
  const [count, setCount] = useState(0);

  useEffect(() => {
    function reportHeight() {
      window.parent.postMessage(
        { type: 'RESIZE_IFRAME', height: document.documentElement.scrollHeight },
        '*'
      );
    }
    reportHeight();
    window.addEventListener('resize', reportHeight);
    return () => window.removeEventListener('resize', reportHeight);
  });

  return <main>count is {count}</main>;
}
