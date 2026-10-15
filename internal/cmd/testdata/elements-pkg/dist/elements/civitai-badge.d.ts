import { type PropertyDeclarations, type TemplateResult } from 'lit';
import { CivitaiElement } from './base.js';
export type BadgeVariant = 'filled' | 'light' | 'outline';
export type BadgeSize = 'sm' | 'md' | 'lg';
export type Intent = 'info' | 'success' | 'warning' | 'error';
export declare class CivitaiBadge extends CivitaiElement {
    static styles: import("lit").CSSResult[];
    static properties: PropertyDeclarations;
    variant: BadgeVariant;
    size: BadgeSize;
    color: Intent | '';
    constructor();
    render(): TemplateResult;
}
export declare function defineCivitaiBadge(): void;
declare global {
    interface HTMLElementTagNameMap {
        'civitai-badge': CivitaiBadge;
    }
}
