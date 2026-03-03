import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import App from './App';

describe('App component', () => {
  it('renders the login view when unauthenticated', async () => {
    render(<App />);
    expect(await screen.findByText('Universal Email Analytics')).toBeDefined();
    expect(screen.getByText('Sign In')).toBeDefined();
  });
});
