import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import es from "./locales/es";
import { gridMove, isTyping, shortcutGroups } from "./shortcuts";

// Minimal stand-ins for the DOM classes isTyping checks with instanceof.
class FakeElement {
  isContentEditable = false;
}
class FakeInput extends FakeElement {
  constructor(public type = "text") {
    super();
  }
}
class FakeTextArea extends FakeElement {}
class FakeSelect extends FakeElement {}

const target = (el: FakeElement) => el as unknown as EventTarget;

beforeAll(() => {
  vi.stubGlobal("HTMLElement", FakeElement);
  vi.stubGlobal("HTMLInputElement", FakeInput);
  vi.stubGlobal("HTMLTextAreaElement", FakeTextArea);
  vi.stubGlobal("HTMLSelectElement", FakeSelect);
});
afterAll(() => vi.unstubAllGlobals());

describe("isTyping", () => {
  it("is false for null and non-elements", () => {
    expect(isTyping(null)).toBe(false);
    expect(isTyping({} as EventTarget)).toBe(false);
  });

  it("is true for text-like inputs, textareas, selects and contenteditable", () => {
    for (const type of ["text", "search", "password", "email", "number"]) {
      expect(isTyping(target(new FakeInput(type)))).toBe(true);
    }
    expect(isTyping(target(new FakeTextArea()))).toBe(true);
    expect(isTyping(target(new FakeSelect()))).toBe(true);
    const div = new FakeElement();
    div.isContentEditable = true;
    expect(isTyping(target(div))).toBe(true);
  });

  it("is false for inputs that take no text and for plain elements", () => {
    for (const type of ["checkbox", "radio", "range", "button", "submit"]) {
      expect(isTyping(target(new FakeInput(type)))).toBe(false);
    }
    expect(isTyping(target(new FakeElement()))).toBe(false);
  });
});

describe("gridMove", () => {
  // 10 items in 4 columns: rows are 0-3, 4-7 and a partial 8-9.
  const move = (from: number, key: string) => gridMove(from, key, 4, 10, 2);

  it("returns null for keys that are not grid keys", () => {
    expect(move(3, "a")).toBeNull();
    expect(move(3, "Enter")).toBeNull();
  });

  it("moves one cell or one row", () => {
    expect(move(5, "ArrowLeft")).toBe(4);
    expect(move(5, "ArrowRight")).toBe(6);
    expect(move(5, "ArrowUp")).toBe(1);
    expect(move(1, "ArrowDown")).toBe(5);
  });

  it("clamps at the edges", () => {
    expect(move(0, "ArrowLeft")).toBe(0);
    expect(move(9, "ArrowRight")).toBe(9);
    expect(move(2, "ArrowUp")).toBe(0);
    expect(move(9, "ArrowDown")).toBe(9);
  });

  it("lands on the last card when going down into a partial row", () => {
    expect(move(7, "ArrowDown")).toBe(9);
    expect(move(6, "ArrowDown")).toBe(9);
  });

  it("pages by pageRows rows and clamps", () => {
    expect(gridMove(0, "PageDown", 4, 100, 3)).toBe(12);
    expect(gridMove(20, "PageUp", 4, 100, 3)).toBe(8);
    expect(move(1, "PageUp")).toBe(0);
    expect(move(5, "PageDown")).toBe(9);
  });

  it("jumps to the first and last item", () => {
    expect(move(5, "Home")).toBe(0);
    expect(move(5, "End")).toBe(9);
  });
});

describe("shortcutGroups", () => {
  it("only references existing message keys", () => {
    for (const g of shortcutGroups) {
      expect(es).toHaveProperty([g.title]);
      for (const item of g.items) {
        expect(item.keys.length).toBeGreaterThan(0);
        expect(es).toHaveProperty([item.label]);
      }
    }
  });
});
