import eslint from '@eslint/js';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  { ignores: ['dist/**', 'public/**'] },
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}', '*.config.ts'],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    rules: {
      // Generated SDK option types currently require a small number of
      // compatibility casts; TypeScript still validates their call sites.
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
);
