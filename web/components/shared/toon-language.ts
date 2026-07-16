// TOON TextMate grammar adapted from VishalRaut2106/vscode-toon.
// Source: https://github.com/VishalRaut2106/vscode-toon/blob/main/syntaxes/toon.tmLanguage.json
// License: MIT, Copyright (c) 2025 Vishal Raut.
export const toonLanguage = {
  name: 'toon',
  displayName: 'TOON',
  scopeName: 'source.toon',
  patterns: [
    { include: '#comments' },
    { include: '#array-header' },
    { include: '#list-item' },
    { include: '#key-value' },
    { include: '#values' },
  ],
  repository: {
    comments: {
      patterns: [
        {
          name: 'comment.line.number-sign.toon',
          match: '#.*$',
        },
      ],
    },
    'array-header': {
      patterns: [
        {
          name: 'meta.array-header.toon',
          match: '^(\\s*)([\\w."]+)?\\[(#)?(\\d+)([,\\t|])?\\](\\{([^}]+)\\})?(:)',
          captures: {
            '2': { name: 'variable.other.property.toon' },
            '3': { name: 'keyword.operator.length-marker.toon' },
            '4': { name: 'constant.numeric.array-length.toon' },
            '5': { name: 'keyword.operator.delimiter.toon' },
            '7': { name: 'entity.name.type.fields.toon' },
            '8': { name: 'punctuation.separator.colon.toon' },
          },
        },
      ],
    },
    'list-item': {
      patterns: [
        {
          name: 'meta.list-item.toon',
          match: '^(\\s*)(-)(\\s+)',
          captures: {
            '2': { name: 'punctuation.definition.list.toon' },
          },
        },
      ],
    },
    'key-value': {
      patterns: [
        {
          name: 'meta.key-value.toon',
          match: '^(\\s*)("[^"]+"|[\\w._-]+)(:)',
          captures: {
            '2': { name: 'variable.other.property.toon' },
            '3': { name: 'punctuation.separator.colon.toon' },
          },
        },
      ],
    },
    values: {
      patterns: [
        { include: '#string' },
        { include: '#number' },
        { include: '#boolean' },
        { include: '#null' },
      ],
    },
    string: {
      patterns: [
        {
          name: 'string.quoted.double.toon',
          begin: '"',
          end: '"',
          patterns: [
            {
              name: 'constant.character.escape.toon',
              match: '\\\\(["\\\\nrt])',
            },
          ],
        },
      ],
    },
    number: {
      patterns: [
        {
          name: 'constant.numeric.toon',
          match: '\\b-?\\d+(\\.\\d+)?([eE][+-]?\\d+)?\\b',
        },
      ],
    },
    boolean: {
      patterns: [
        {
          name: 'constant.language.boolean.toon',
          match: '\\b(true|false)\\b',
        },
      ],
    },
    null: {
      patterns: [
        {
          name: 'constant.language.null.toon',
          match: '\\bnull\\b',
        },
      ],
    },
  },
}
