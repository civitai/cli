import { nothing, type PropertyDeclarations, type PropertyValues, type TemplateResult } from 'lit';
import { CivitaiElement } from './base.js';
export type ModalSize = 'sm' | 'md' | 'lg';
/**
 * Native `<dialog>.showModal()`: the focus trap, the top layer, the inert
 * background and focus restore all come from the platform. The React binding
 * documents that it traps nothing, so this is a strict upgrade, not a port.
 */
export declare class CivitaiModal extends CivitaiElement {
    #private;
    static styles: import("lit").CSSResult[];
    static properties: PropertyDeclarations;
    open: boolean;
    /** Not `title`: that attribute is global and would render a tooltip. */
    heading: string;
    size: ModalSize;
    withCloseButton: boolean;
    closeOnOverlayClick: boolean;
    closeOnEscape: boolean;
    closeLabel: string;
    constructor();
    show(): void;
    hide(): void;
    toggle(): void;
    protected updated(changed: PropertyValues): void;
    render(): TemplateResult;
    protected renderBody(): TemplateResult;
    /** `<civitai-confirm-dialog>` is this with its two buttons filled in. */
    protected renderFooter(): TemplateResult | typeof nothing;
}
export declare function defineCivitaiModal(): void;
declare global {
    interface HTMLElementTagNameMap {
        'civitai-modal': CivitaiModal;
    }
}
