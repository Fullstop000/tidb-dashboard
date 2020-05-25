import React, { useCallback } from 'react'
import Highlighter from 'react-highlight-words'
import { TextWrap, Pre } from '@lib/components'

import styles from './Styles.module.css'

interface LogProps {
  patterns: string[]
  expanded: boolean
  log: string
}

export default function Log({ patterns, log, expanded }: LogProps) {
  const handleClick = useCallback((ev: React.MouseEvent<HTMLDivElement>) => {
    ev.stopPropagation()
  }, [])
  return (
    <TextWrap
      multiline={expanded}
      onClick={handleClick}
      className={styles.logText}
    >
      <Pre>
        <Highlighter
          searchWords={patterns.map((p) => new RegExp(p, 'gi'))}
          textToHighlight={log}
        />
      </Pre>
    </TextWrap>
  )
}
