import { useState, type CSSProperties, type InputHTMLAttributes } from 'react'

/** Password field with a show / hide button (eye). */
export function PasswordInput({ style, ...props }: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & { style?: CSSProperties }) {
  const [show, setShow] = useState(false)
  return (
    <span className="pw">
      <input {...props} type={show ? 'text' : 'password'} style={{ ...style, paddingRight: 40 }} />
      <button type="button" className="pw-eye" onClick={() => setShow(!show)} aria-label={show ? 'Sembunyikan kata sandi' : 'Lihat kata sandi'} title={show ? 'Sembunyikan kata sandi' : 'Lihat kata sandi'} aria-pressed={show}>
        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12Z" />
          <circle cx="12" cy="12" r="3" />
          {show && <path d="M3 3l18 18" />}
        </svg>
      </button>
    </span>
  )
}
