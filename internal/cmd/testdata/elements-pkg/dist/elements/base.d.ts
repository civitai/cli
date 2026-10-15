import { LitElement, type PropertyValues } from 'lit';
/**
 * Deepest focused node, crossing every shadow boundary. `document.activeElement`
 * stops at the outermost host, which is never the node that actually has focus.
 */
export declare function deepActiveElement(root?: DocumentOrShadowRoot): Element | null;
export declare abstract class CivitaiElement extends LitElement {
    connectedCallback(): void;
    protected firstUpdated(changed: PropertyValues): void;
}
