import { defineConfig } from "oxlint";

import confusingBrowserGlobals from "confusing-browser-globals";

export default defineConfig({
  plugins: ["eslint", "jsx-a11y", "react", "typescript"],
  ignorePatterns: ["dist/**/*"],
  env: {
    builtin: true,
  },
  categories: {
    correctness: "error",
  },
  rules: {
    "no-array-constructor": "error",
    "no-case-declarations": "error",
    "no-empty": "error",
    "no-fallthrough": "error",
    "no-prototype-builtins": "error",
    "no-redeclare": "error",
    "no-regex-spaces": "error",
    "no-restricted-globals": ["error", { globals: confusingBrowserGlobals }],
    "no-unexpected-multiline": "error",
    "no-var": "error",
    "prefer-const": "error",
    "prefer-rest-params": "error",
    "prefer-spread": "error",
    "react/display-name": "error",
    "react/jsx-no-comment-textnodes": "error",
    "react/jsx-no-target-blank": "error",
    "react/no-unescaped-entities": "error",
    "react/no-unknown-property": "error",
    "react/rules-of-hooks": "error",
    "typescript/ban-ts-comment": "error",
    "typescript/no-empty-object-type": "error",
    "typescript/no-explicit-any": "error",
    "typescript/no-require-imports": "error",
    "typescript/no-unnecessary-type-constraint": "error",
  },
});