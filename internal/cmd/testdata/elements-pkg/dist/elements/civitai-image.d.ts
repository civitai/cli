import { type PropertyDeclarations, type TemplateResult } from 'lit';
import { CivitaiMediaElement } from './media-base.js';
export type ImageFit = 'cover' | 'contain';
export type ImageStatus = 'loading' | 'loaded' | 'error' | 'blocked';
/** `<img loading>`'s values. `''` means the attribute is not rendered at all. */
export type ImageLoading = 'eager' | 'lazy' | '';
/** `<img decoding>`'s values. `''` means the attribute is not rendered at all. */
export type ImageDecoding = 'sync' | 'async' | 'auto' | '';
export declare class CivitaiImage extends CivitaiMediaElement {
    #private;
    static styles: import("lit").CSSResultGroup[];
    static properties: PropertyDeclarations;
    fit: ImageFit;
    /** The image becomes a button that emits `open`, for a viewer or a detail page. */
    openable: boolean;
    /** Names the open button when it should say more than `alt`. */
    label: string;
    /**
     * `<img loading>`, passed straight through: `lazy` defers a tile that is still
     * below the fold. Unset renders no attribute, so the default stays HTML's.
     * The `<img>` is built in this element's shadow root, so a consumer's markup
     * cannot reach it — which is why this is a property rather than nothing.
     */
    loading: ImageLoading;
    /** `<img decoding>`, passed straight through. Unset renders no attribute. */
    decoding: ImageDecoding;
    constructor();
    protected renderMedia(): TemplateResult;
}
export declare function defineCivitaiImage(): void;
declare global {
    interface HTMLElementTagNameMap {
        'civitai-image': CivitaiImage;
    }
}
