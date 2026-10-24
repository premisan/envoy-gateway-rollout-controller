import { useState, useEffect, useRef } from 'react';

export function parseDurationToSeconds(str: string): number {
  if (!str) return 0;
  let total = 0;
  const mMatch = str.match(/(\d+)\s*m/);
  if (mMatch) total += parseInt(mMatch[1], 10) * 60;
  const sMatch = str.match(/(\d+)\s*s/);
  if (sMatch) total += parseInt(sMatch[1], 10);
  return total;
}

export function formatSecondsToDuration(sec: number): string {
  if (sec <= 0) return '0s';
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

export function useCountdownTimer(rawRemaining?: string, isActive = true): string {
  const [seconds, setSeconds] = useState<number>(() => parseDurationToSeconds(rawRemaining || ''));
  const rawRemainingRef = useRef<string | undefined>(rawRemaining);

  // Sync with backend authority whenever rawRemaining updates
  useEffect(() => {
    if (rawRemaining !== rawRemainingRef.current) {
      rawRemainingRef.current = rawRemaining;
      const parsed = parseDurationToSeconds(rawRemaining || '');
      setSeconds(parsed);
    }
  }, [rawRemaining]);

  // 1-second interval countdown
  useEffect(() => {
    if (!isActive || seconds <= 0) return;

    const interval = setInterval(() => {
      setSeconds((prev) => {
        if (prev <= 1) {
          clearInterval(interval);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [isActive, seconds > 0]);

  if (!isActive || !rawRemaining || rawRemaining === '0s') {
    return '0s';
  }

  return formatSecondsToDuration(seconds);
}
