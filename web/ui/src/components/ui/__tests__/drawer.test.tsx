import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Drawer, DrawerContent, DrawerTitle } from '../drawer';

describe('Drawer layering', () => {
  it('portals its backdrop and content above the application chrome', () => {
    render(
      <div data-testid="app-chrome">
        <Drawer direction="right" open>
          <DrawerContent>
            <DrawerTitle>Test drawer</DrawerTitle>
          </DrawerContent>
        </Drawer>
      </div>,
    );

    const content = screen.getByRole('dialog', { name: 'Test drawer' });
    const overlay = document.querySelector('[data-slot="drawer-overlay"]');

    expect(content).toHaveAttribute('data-vaul-drawer-direction', 'right');
    expect(content).toHaveClass('z-[101]');
    expect(content).toHaveClass('bg-white');
    expect(overlay).toHaveClass('z-[100]');
    expect(content.parentElement).toBe(document.body);
  });
});
