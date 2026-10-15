import { type PropertyDeclarations, type TemplateResult } from 'lit';
import { CivitaiElement } from './base.js';
import type { Intent } from './civitai-badge.js';
export type { Intent } from './civitai-badge.js';
export type ButtonVariant = 'filled' | 'light' | 'outline' | 'subtle';
export type ButtonSize = 'sm' | 'md' | 'lg';
export declare class CivitaiButton extends CivitaiElement {
    #private;
    static formAssociated: boolean;
    static styles: import("lit").CSSResult[];
    static properties: PropertyDeclarations;
    /** Visual style. */
    variant: ButtonVariant;
    /** Recolours every variant; absent keeps the primary accent. */
    color: Intent | '';
    /** Renders an anchor instead. Navigation is a link, whatever it looks like. */
    href: string;
    /** Size preset. */
    size: ButtonSize;
    /** Shows a spinner and, like the React binding, disables the control. */
    loading: boolean;
    disabled: boolean;
    fullWidth: boolean;
    /** Matches the native attribute, including its `submit`/`reset` behaviour. */
    type: 'button' | 'submit' | 'reset';
    constructor();
    /** The form this button would submit, exactly as a native button reports it. */
    get form(): HTMLFormElement | null;
    connectedCallback(): void;
    disconnectedCallback(): void;
    render(): TemplateResult;
}
export declare function defineCivitaiButton(): void;
declare global {
    interface HTMLElementTagNameMap {
        'civitai-button': CivitaiButton;
    }
}
