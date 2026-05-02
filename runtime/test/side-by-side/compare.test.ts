import { describe, it, expect } from "vitest";
import { parseFrontmatter } from "./compare";

describe("parseFrontmatter", () => {
  it("extracts name/slug/difficulty/agentType and body", () => {
    const text = `---
title: "Add a section"
slug: 001-add-section
difficulty: easy
agentType: code-pr
---
Body text here.

Multiple paragraphs.
`;
    const r = parseFrontmatter(text);
    expect(r).not.toBeNull();
    expect(r!.frontmatter.title).toBe("Add a section");
    expect(r!.frontmatter.slug).toBe("001-add-section");
    expect(r!.frontmatter.difficulty).toBe("easy");
    expect(r!.frontmatter.agentType).toBe("code-pr");
    expect(r!.body).toContain("Body text here.");
    expect(r!.body).toContain("Multiple paragraphs.");
  });

  it("returns null when no frontmatter delimiter", () => {
    expect(parseFrontmatter("just a body, no delimiters")).toBeNull();
  });

  it("strips wrapping quotes from values", () => {
    const r = parseFrontmatter(`---
title: 'single quoted'
slug: x
---
b`);
    expect(r!.frontmatter.title).toBe("single quoted");
  });

  it("defaults agentType to code-pr when omitted", () => {
    const r = parseFrontmatter(`---
title: t
slug: s
difficulty: easy
---
b`);
    expect(r!.frontmatter.agentType).toBe("code-pr");
  });
});
