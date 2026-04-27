[![logo by @sawaratsuki1004](https://react.dev/_next/image?url=%2Fimages%2Fuwu.png&w=128&q=75)](https://react.dev/)

[React](https://react.dev/)

[v19.2](https://react.dev/versions)

Search`⌘`  `Ctrl`  `K`

[Learn](https://react.dev/learn)

[Reference](https://react.dev/reference/react)

[Community](https://react.dev/community)

[Blog](https://react.dev/blog)

[Translations](https://react.dev/community/translations)

[Open on GitHub](https://github.com/facebook/react/releases)

[Blog](https://react.dev/blog)

Copy pageCopy

# React 19 Upgrade Guide [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#undefined "Link for this heading")

April 25, 2024 by [Ricky Hanlon](https://twitter.com/rickhanlonii)

* * *

The improvements added to React 19 require some breaking changes, but we’ve worked to make the upgrade as smooth as possible, and we don’t expect the changes to impact most apps.

### Note

#### React 18.3 has also been published [Link for React 18.3 has also been published ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#react-18-3 "Link for React 18.3 has also been published ")

To help make the upgrade to React 19 easier, we’ve published a `react@18.3` release that is identical to 18.2 but adds warnings for deprecated APIs and other changes that are needed for React 19.

We recommend upgrading to React 18.3 first to help identify any issues before upgrading to React 19.

For a list of changes in 18.3 see the [Release Notes](https://github.com/facebook/react/blob/main/CHANGELOG.md#1830-april-25-2024).

In this post, we will guide you through the steps for upgrading to React 19:

- [Installing](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#installing)
- [Codemods](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#codemods)
- [Breaking changes](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#breaking-changes)
- [New deprecations](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#new-deprecations)
- [Notable changes](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#notable-changes)
- [TypeScript changes](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#typescript-changes)
- [Changelog](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#changelog)

If you’d like to help us test React 19, follow the steps in this upgrade guide and [report any issues](https://github.com/facebook/react/issues/new?assignees=&labels=React+19&projects=&template=19.md&title=%5BReact+19%5D) you encounter. For a list of new features added to React 19, see the [React 19 release post](https://react.dev/blog/2024/12/05/react-19).

* * *

## Installing [Link for Installing ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#installing "Link for Installing ")

### Note

#### New JSX Transform is now required [Link for New JSX Transform is now required ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#new-jsx-transform-is-now-required "Link for New JSX Transform is now required ")

We introduced a [new JSX transform](https://legacy.reactjs.org/blog/2020/09/22/introducing-the-new-jsx-transform.html) in 2020 to improve bundle size and use JSX without importing React. In React 19, we’re adding additional improvements like using ref as a prop and JSX speed improvements that require the new transform.

If the new transform is not enabled, you will see this warning:

Console

Your app (or one of its dependencies) is using an outdated JSX transform. Update to the modern JSX transform for faster performance: [https://react.dev/link/new-jsx-transform](https://react.dev/link/new-jsx-transform)

We expect most apps will not be affected since the transform is enabled in most environments already. For manual instructions on how to upgrade, please see the [announcement post](https://legacy.reactjs.org/blog/2020/09/22/introducing-the-new-jsx-transform.html).

To install the latest version of React and React DOM:

```
npm install --save-exact react@^19.0.0 react-dom@^19.0.0
```

Or, if you’re using Yarn:

```
yarn add --exact react@^19.0.0 react-dom@^19.0.0
```

If you’re using TypeScript, you also need to update the types.

```
npm install --save-exact @types/react@^19.0.0 @types/react-dom@^19.0.0
```

Or, if you’re using Yarn:

```
yarn add --exact @types/react@^19.0.0 @types/react-dom@^19.0.0
```

We’re also including a codemod for the most common replacements. See [TypeScript changes](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#typescript-changes) below.

## Codemods [Link for Codemods ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#codemods "Link for Codemods ")

To help with the upgrade, we’ve worked with the team at [codemod.com](https://codemod.com/) to publish codemods that will automatically update your code to many of the new APIs and patterns in React 19.

All codemods are available in the [`react-codemod` repo](https://github.com/reactjs/react-codemod) and the Codemod team have joined in helping maintain the codemods. To run these codemods, we recommend using the `codemod` command instead of the `react-codemod` because it runs faster, handles more complex code migrations, and provides better support for TypeScript.

### Note

#### Run all React 19 codemods [Link for Run all React 19 codemods ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#run-all-react-19-codemods "Link for Run all React 19 codemods ")

Run all codemods listed in this guide with the React 19 `codemod` recipe:

```
npx codemod@latest react/19/migration-recipe
```

This will run the following codemods from `react-codemod`:

- [`replace-reactdom-render`](https://github.com/reactjs/react-codemod?tab=readme-ov-file#replace-reactdom-render)
- [`replace-string-ref`](https://github.com/reactjs/react-codemod?tab=readme-ov-file#replace-string-ref)
- [`replace-act-import`](https://github.com/reactjs/react-codemod?tab=readme-ov-file#replace-act-import)
- [`replace-use-form-state`](https://github.com/reactjs/react-codemod?tab=readme-ov-file#replace-use-form-state)
- [`prop-types-typescript`](https://github.com/reactjs/react-codemod#react-proptypes-to-prop-types)

This does not include the TypeScript changes. See [TypeScript changes](https://react.dev/blog/2024/04/25/react-19-upgrade-guide#typescript-changes) below.

Changes that include a codemod include the command below.

For a list of all available codemods, see the [`react-codemod` repo](https://github.com/reactjs/react-codemod).

## Breaking changes [Link for Breaking changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#breaking-changes "Link for Breaking changes ")

### Errors in render are not re-thrown [Link for Errors in render are not re-thrown ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#errors-in-render-are-not-re-thrown "Link for Errors in render are not re-thrown ")

In previous versions of React, errors thrown during render were caught and rethrown. In DEV, we would also log to `console.error`, resulting in duplicate error logs.

In React 19, we’ve [improved how errors are handled](https://react.dev/blog/2024/04/25/react-19#error-handling) to reduce duplication by not re-throwing:

- **Uncaught Errors**: Errors that are not caught by an Error Boundary are reported to `window.reportError`.
- **Caught Errors**: Errors that are caught by an Error Boundary are reported to `console.error`.

This change should not impact most apps, but if your production error reporting relies on errors being re-thrown, you may need to update your error handling. To support this, we’ve added new methods to `createRoot` and `hydrateRoot` for custom error handling:

```
const root = createRoot(container, {

  onUncaughtError: (error, errorInfo) => {

    // ... log error report

  },

  onCaughtError: (error, errorInfo) => {

    // ... log error report

  }

});
```

For more info, see the docs for [`createRoot`](https://react.dev/reference/react-dom/client/createRoot) and [`hydrateRoot`](https://react.dev/reference/react-dom/client/hydrateRoot).

### Removed deprecated React APIs [Link for Removed deprecated React APIs ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-deprecated-react-apis "Link for Removed deprecated React APIs ")

#### Removed: `propTypes` and `defaultProps` for functions [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-proptypes-and-defaultprops "Link for this heading")

`PropTypes` were deprecated in [April 2017 (v15.5.0)](https://legacy.reactjs.org/blog/2017/04/07/react-v15.5.0.html#new-deprecation-warnings).

In React 19, we’re removing the `propType` checks from the React package, and using them will be silently ignored. If you’re using `propTypes`, we recommend migrating to TypeScript or another type-checking solution.

We’re also removing `defaultProps` from function components in place of ES6 default parameters. Class components will continue to support `defaultProps` since there is no ES6 alternative.

```
// Before

import PropTypes from 'prop-types';

function Heading({text}) {

  return <h1>{text}</h1>;

}

Heading.propTypes = {

  text: PropTypes.string,

};

Heading.defaultProps = {

  text: 'Hello, world!',

};
```

```
// After

interface Props {

  text?: string;

}

function Heading({text = 'Hello, world!'}: Props) {

  return <h1>{text}</h1>;

}
```

### Note

Codemod `propTypes` to TypeScript with:

```
npx codemod@latest react/prop-types-typescript
```

#### Removed: Legacy Context using `contextTypes` and `getChildContext` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-removing-legacy-context "Link for this heading")

Legacy Context was deprecated in [October 2018 (v16.6.0)](https://legacy.reactjs.org/blog/2018/10/23/react-v-16-6.html).

Legacy Context was only available in class components using the APIs `contextTypes` and `getChildContext`, and was replaced with `contextType` due to subtle bugs that were easy to miss. In React 19, we’re removing Legacy Context to make React slightly smaller and faster.

If you’re still using Legacy Context in class components, you’ll need to migrate to the new `contextType` API:

```
// Before

import PropTypes from 'prop-types';

class Parent extends React.Component {

  static childContextTypes = {

    foo: PropTypes.string.isRequired,

  };

  getChildContext() {

    return { foo: 'bar' };

  }

  render() {

    return <Child />;

  }

}

class Child extends React.Component {

  static contextTypes = {

    foo: PropTypes.string.isRequired,

  };

  render() {

    return <div>{this.context.foo}</div>;

  }

}
```

```
// After

const FooContext = React.createContext();

class Parent extends React.Component {

  render() {

    return (

      <FooContext value='bar'>

        <Child />

      </FooContext>

    );

  }

}

class Child extends React.Component {

  static contextType = FooContext;

  render() {

    return <div>{this.context}</div>;

  }

}
```

#### Removed: string refs [Link for Removed: string refs ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-string-refs "Link for Removed: string refs ")

String refs were deprecated in [March, 2018 (v16.3.0)](https://legacy.reactjs.org/blog/2018/03/27/update-on-async-rendering.html).

Class components supported string refs before being replaced by ref callbacks due to [multiple downsides](https://github.com/facebook/react/issues/1373). In React 19, we’re removing string refs to make React simpler and easier to understand.

If you’re still using string refs in class components, you’ll need to migrate to ref callbacks:

```
// Before

class MyComponent extends React.Component {

  componentDidMount() {

    this.refs.input.focus();

  }

  render() {

    return <input ref='input' />;

  }

}
```

```
// After

class MyComponent extends React.Component {

  componentDidMount() {

    this.input.focus();

  }

  render() {

    return <input ref={input => this.input = input} />;

  }

}
```

### Note

Codemod string refs with `ref` callbacks:

```
npx codemod@latest react/19/replace-string-ref
```

#### Removed: Module pattern factories [Link for Removed: Module pattern factories ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-module-pattern-factories "Link for Removed: Module pattern factories ")

Module pattern factories were deprecated in [August 2019 (v16.9.0)](https://legacy.reactjs.org/blog/2019/08/08/react-v16.9.0.html#deprecating-module-pattern-factories).

This pattern was rarely used and supporting it causes React to be slightly larger and slower than necessary. In React 19, we’re removing support for module pattern factories, and you’ll need to migrate to regular functions:

```
// Before

function FactoryComponent() {

  return { render() { return <div />; } }

}
```

```
// After

function FactoryComponent() {

  return <div />;

}
```

#### Removed: `React.createFactory` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-createfactory "Link for this heading")

`createFactory` was deprecated in [February 2020 (v16.13.0)](https://legacy.reactjs.org/blog/2020/02/26/react-v16.13.0.html#deprecating-createfactory).

Using `createFactory` was common before broad support for JSX, but it’s rarely used today and can be replaced with JSX. In React 19, we’re removing `createFactory` and you’ll need to migrate to JSX:

```
// Before

import { createFactory } from 'react';

const button = createFactory('button');
```

```
// After

const button = <button />;
```

#### Removed: `react-test-renderer/shallow` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-react-test-renderer-shallow "Link for this heading")

In React 18, we updated `react-test-renderer/shallow` to re-export [react-shallow-renderer](https://github.com/enzymejs/react-shallow-renderer). In React 19, we’re removing `react-test-render/shallow` to prefer installing the package directly:

```
npm install react-shallow-renderer --save-dev
```

```
- import ShallowRenderer from 'react-test-renderer/shallow';

+ import ShallowRenderer from 'react-shallow-renderer';
```

### Note

##### Please reconsider shallow rendering [Link for Please reconsider shallow rendering ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#please-reconsider-shallow-rendering "Link for Please reconsider shallow rendering ")

Shallow rendering depends on React internals and can block you from future upgrades. We recommend migrating your tests to [@testing-library/react](https://testing-library.com/docs/react-testing-library/intro/) or [@testing-library/react-native](https://testing-library.com/docs/react-native-testing-library/intro).

### Removed deprecated React DOM APIs [Link for Removed deprecated React DOM APIs ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-deprecated-react-dom-apis "Link for Removed deprecated React DOM APIs ")

#### Removed: `react-dom/test-utils` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-react-dom-test-utils "Link for this heading")

We’ve moved `act` from `react-dom/test-utils` to the `react` package:

Console

`ReactDOMTestUtils.act` is deprecated in favor of `React.act`. Import `act` from `react` instead of `react-dom/test-utils`. See [https://react.dev/warnings/react-dom-test-utils](https://react.dev/warnings/react-dom-test-utils) for more info.

To fix this warning, you can import `act` from `react`:

```
- import {act} from 'react-dom/test-utils'

+ import {act} from 'react';
```

All other `test-utils` functions have been removed. These utilities were uncommon, and made it too easy to depend on low level implementation details of your components and React. In React 19, these functions will error when called and their exports will be removed in a future version.

See the [warning page](https://react.dev/warnings/react-dom-test-utils) for alternatives.

### Note

Codemod `ReactDOMTestUtils.act` to `React.act`:

```
npx codemod@latest react/19/replace-act-import
```

#### Removed: `ReactDOM.render` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-reactdom-render "Link for this heading")

`ReactDOM.render` was deprecated in [March 2022 (v18.0.0)](https://react.dev/blog/2022/03/08/react-18-upgrade-guide). In React 19, we’re removing `ReactDOM.render` and you’ll need to migrate to using [`ReactDOM.createRoot`](https://react.dev/reference/react-dom/client/createRoot):

```
// Before

import {render} from 'react-dom';

render(<App />, document.getElementById('root'));

// After

import {createRoot} from 'react-dom/client';

const root = createRoot(document.getElementById('root'));

root.render(<App />);
```

### Note

Codemod `ReactDOM.render` to `ReactDOMClient.createRoot`:

```
npx codemod@latest react/19/replace-reactdom-render
```

#### Removed: `ReactDOM.hydrate` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-reactdom-hydrate "Link for this heading")

`ReactDOM.hydrate` was deprecated in [March 2022 (v18.0.0)](https://react.dev/blog/2022/03/08/react-18-upgrade-guide). In React 19, we’re removing `ReactDOM.hydrate` you’ll need to migrate to using [`ReactDOM.hydrateRoot`](https://react.dev/reference/react-dom/client/hydrateRoot),

```
// Before

import {hydrate} from 'react-dom';

hydrate(<App />, document.getElementById('root'));

// After

import {hydrateRoot} from 'react-dom/client';

hydrateRoot(document.getElementById('root'), <App />);
```

### Note

Codemod `ReactDOM.hydrate` to `ReactDOMClient.hydrateRoot`:

```
npx codemod@latest react/19/replace-reactdom-render
```

#### Removed: `unmountComponentAtNode` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-unmountcomponentatnode "Link for this heading")

`ReactDOM.unmountComponentAtNode` was deprecated in [March 2022 (v18.0.0)](https://react.dev/blog/2022/03/08/react-18-upgrade-guide). In React 19, you’ll need to migrate to using `root.unmount()`.

```
// Before

unmountComponentAtNode(document.getElementById('root'));

// After

root.unmount();
```

For more see `root.unmount()` for [`createRoot`](https://react.dev/reference/react-dom/client/createRoot#root-unmount) and [`hydrateRoot`](https://react.dev/reference/react-dom/client/hydrateRoot#root-unmount).

### Note

Codemod `unmountComponentAtNode` to `root.unmount`:

```
npx codemod@latest react/19/replace-reactdom-render
```

#### Removed: `ReactDOM.findDOMNode` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-reactdom-finddomnode "Link for this heading")

`ReactDOM.findDOMNode` was [deprecated in October 2018 (v16.6.0)](https://legacy.reactjs.org/blog/2018/10/23/react-v-16-6.html#deprecations-in-strictmode).

We’re removing `findDOMNode` because it was a legacy escape hatch that was slow to execute, fragile to refactoring, only returned the first child, and broke abstraction levels (see more [here](https://legacy.reactjs.org/docs/strict-mode.html#warning-about-deprecated-finddomnode-usage)). You can replace `ReactDOM.findDOMNode` with [DOM refs](https://react.dev/learn/manipulating-the-dom-with-refs):

```
// Before

import {findDOMNode} from 'react-dom';

function AutoselectingInput() {

  useEffect(() => {

    const input = findDOMNode(this);

    input.select()

  }, []);

  return <input defaultValue="Hello" />;

}
```

```
// After

function AutoselectingInput() {

  const ref = useRef(null);

  useEffect(() => {

    ref.current.select();

  }, []);

  return <input ref={ref} defaultValue="Hello" />

}
```

## New deprecations [Link for New deprecations ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#new-deprecations "Link for New deprecations ")

### Deprecated: `element.ref` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#deprecated-element-ref "Link for this heading")

React 19 supports [`ref` as a prop](https://react.dev/blog/2024/04/25/react-19#ref-as-a-prop), so we’re deprecating the `element.ref` in place of `element.props.ref`.

Accessing `element.ref` will warn:

Console

Accessing element.ref is no longer supported. ref is now a regular prop. It will be removed from the JSX Element type in a future release.

### Deprecated: `react-test-renderer` [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#deprecated-react-test-renderer "Link for this heading")

We are deprecating `react-test-renderer` because it implements its own renderer environment that doesn’t match the environment users use, promotes testing implementation details, and relies on introspection of React’s internals.

The test renderer was created before there were more viable testing strategies available like [React Testing Library](https://testing-library.com/), and we now recommend using a modern testing library instead.

In React 19, `react-test-renderer` logs a deprecation warning, and has switched to concurrent rendering. We recommend migrating your tests to [@testing-library/react](https://testing-library.com/docs/react-testing-library/intro/) or [@testing-library/react-native](https://testing-library.com/docs/react-native-testing-library/intro) for a modern and well supported testing experience.

## Notable changes [Link for Notable changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#notable-changes "Link for Notable changes ")

### StrictMode changes [Link for StrictMode changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#strict-mode-improvements "Link for StrictMode changes ")

React 19 includes several fixes and improvements to Strict Mode.

When double rendering in Strict Mode in development, `useMemo` and `useCallback` will reuse the memoized results from the first render during the second render. Components that are already Strict Mode compatible should not notice a difference in behavior.

As with all Strict Mode behaviors, these features are designed to proactively surface bugs in your components during development so you can fix them before they are shipped to production. For example, during development, Strict Mode will double-invoke ref callback functions on initial mount, to simulate what happens when a mounted component is replaced by a Suspense fallback.

### Improvements to Suspense [Link for Improvements to Suspense ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#improvements-to-suspense "Link for Improvements to Suspense ")

In React 19, when a component suspends, React will immediately commit the fallback of the nearest Suspense boundary without waiting for the entire sibling tree to render. After the fallback commits, React schedules another render for the suspended siblings to “pre-warm” lazy requests in the rest of the tree:

![Diagram showing a tree of three components, one parent labeled Accordion and two children labeled Panel. Both Panel components contain isActive with value false.](https://react.dev/_next/image?url=%2Fimages%2Fdocs%2Fdiagrams%2Fprerender.dark.png&w=3840&q=75)

![Diagram showing a tree of three components, one parent labeled Accordion and two children labeled Panel. Both Panel components contain isActive with value false.](https://react.dev/_next/image?url=%2Fimages%2Fdocs%2Fdiagrams%2Fprerender.png&w=3840&q=75)

Previously, when a component suspended, the suspended siblings were rendered and then the fallback was committed.

![The same diagram as the previous, with the isActive of the first child Panel component highlighted indicating a click with the isActive value set to true. The second Panel component still contains value false.](https://react.dev/_next/image?url=%2Fimages%2Fdocs%2Fdiagrams%2Fprewarm.dark.png&w=3840&q=75)

![The same diagram as the previous, with the isActive of the first child Panel component highlighted indicating a click with the isActive value set to true. The second Panel component still contains value false.](https://react.dev/_next/image?url=%2Fimages%2Fdocs%2Fdiagrams%2Fprewarm.png&w=3840&q=75)

In React 19, when a component suspends, the fallback is committed and then the suspended siblings are rendered.

This change means Suspense fallbacks display faster, while still warming lazy requests in the suspended tree.

### UMD builds removed [Link for UMD builds removed ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#umd-builds-removed "Link for UMD builds removed ")

UMD was widely used in the past as a convenient way to load React without a build step. Now, there are modern alternatives for loading modules as scripts in HTML documents. Starting with React 19, React will no longer produce UMD builds to reduce the complexity of its testing and release process.

To load React 19 with a script tag, we recommend using an ESM-based CDN such as [esm.sh](https://esm.sh/).

```
<script type="module">

  import React from "https://esm.sh/react@19/?dev"

  import ReactDOMClient from "https://esm.sh/react-dom@19/client?dev"

  ...

</script>
```

### Libraries depending on React internals may block upgrades [Link for Libraries depending on React internals may block upgrades ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#libraries-depending-on-react-internals-may-block-upgrades "Link for Libraries depending on React internals may block upgrades ")

This release includes changes to React internals that may impact libraries that ignore our pleas to not use internals like `SECRET_INTERNALS_DO_NOT_USE_OR_YOU_WILL_BE_FIRED`. These changes are necessary to land improvements in React 19, and will not break libraries that follow our guidelines.

Based on our [Versioning Policy](https://react.dev/community/versioning-policy#what-counts-as-a-breaking-change), these updates are not listed as breaking changes, and we are not including docs for how to upgrade them. The recommendation is to remove any code that depends on internals.

To reflect the impact of using internals, we have renamed the `SECRET_INTERNALS` suffix to:

`_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE`

In the future we will more aggressively block accessing internals from React to discourage usage and ensure users are not blocked from upgrading.

## TypeScript changes [Link for TypeScript changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#typescript-changes "Link for TypeScript changes ")

### Removed deprecated TypeScript types [Link for Removed deprecated TypeScript types ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#removed-deprecated-typescript-types "Link for Removed deprecated TypeScript types ")

We’ve cleaned up the TypeScript types based on the removed APIs in React 19. Some of the removed have types been moved to more relevant packages, and others are no longer needed to describe React’s behavior.

### Note

We’ve published [`types-react-codemod`](https://github.com/eps1lon/types-react-codemod/) to migrate most type related breaking changes:

```
npx types-react-codemod@latest preset-19 ./path-to-app
```

If you have a lot of unsound access to `element.props`, you can run this additional codemod:

```
npx types-react-codemod@latest react-element-default-any-props ./path-to-your-react-ts-files
```

Check out [`types-react-codemod`](https://github.com/eps1lon/types-react-codemod/) for a list of supported replacements. If you feel a codemod is missing, it can be tracked in the [list of missing React 19 codemods](https://github.com/eps1lon/types-react-codemod/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc+label%3A%22React+19%22+label%3Aenhancement).

### `ref` cleanups required [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#ref-cleanup-required "Link for this heading")

_This change is included in the `react-19` codemod preset as [`no-implicit-ref-callback-return`](https://github.com/eps1lon/types-react-codemod/#no-implicit-ref-callback-return)._

Due to the introduction of ref cleanup functions, returning anything else from a ref callback will now be rejected by TypeScript. The fix is usually to stop using implicit returns:

```
- <div ref={current => (instance = current)} />

+ <div ref={current => {instance = current}} />
```

The original code returned the instance of the `HTMLDivElement` and TypeScript wouldn’t know if this was supposed to be a cleanup function or not.

### `useRef` requires an argument [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#useref-requires-argument "Link for this heading")

_This change is included in the `react-19` codemod preset as [`refobject-defaults`](https://github.com/eps1lon/types-react-codemod/#refobject-defaults)._

A long-time complaint of how TypeScript and React work has been `useRef`. We’ve changed the types so that `useRef` now requires an argument. This significantly simplifies its type signature. It’ll now behave more like `createContext`.

```
// @ts-expect-error: Expected 1 argument but saw none

useRef();

// Passes

useRef(undefined);

// @ts-expect-error: Expected 1 argument but saw none

createContext();

// Passes

createContext(undefined);
```

This now also means that all refs are mutable. You’ll no longer hit the issue where you can’t mutate a ref because you initialised it with `null`:

```
const ref = useRef<number>(null);

// Cannot assign to 'current' because it is a read-only property

ref.current = 1;
```

`MutableRef` is now deprecated in favor of a single `RefObject` type which `useRef` will always return:

```
interface RefObject<T> {

  current: T

}

declare function useRef<T>: RefObject<T>
```

`useRef` still has a convenience overload for `useRef<T>(null)` that automatically returns `RefObject<T | null>`. To ease migration due to the required argument for `useRef`, a convenience overload for `useRef(undefined)` was added that automatically returns `RefObject<T | undefined>`.

Check out [\[RFC\] Make all refs mutable](https://github.com/DefinitelyTyped/DefinitelyTyped/pull/64772) for prior discussions about this change.

### Changes to the `ReactElement` TypeScript type [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#changes-to-the-reactelement-typescript-type "Link for this heading")

_This change is included in the [`react-element-default-any-props`](https://github.com/eps1lon/types-react-codemod#react-element-default-any-props) codemod._

The `props` of React elements now default to `unknown` instead of `any` if the element is typed as `ReactElement`. This does not affect you if you pass a type argument to `ReactElement`:

```
type Example2 = ReactElement<{ id: string }>["props"];

//   ^? { id: string }
```

But if you relied on the default, you now have to handle `unknown`:

```
type Example = ReactElement["props"];

//   ^? Before, was 'any', now 'unknown'
```

You should only need it if you have a lot of legacy code relying on unsound access of element props. Element introspection only exists as an escape hatch, and you should make it explicit that your props access is unsound via an explicit `any`.

### The JSX namespace in TypeScript [Link for The JSX namespace in TypeScript ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#the-jsx-namespace-in-typescript "Link for The JSX namespace in TypeScript ")

This change is included in the `react-19` codemod preset as [`scoped-jsx`](https://github.com/eps1lon/types-react-codemod#scoped-jsx)

A long-time request is to remove the global `JSX` namespace from our types in favor of `React.JSX`. This helps prevent pollution of global types which prevents conflicts between different UI libraries that leverage JSX.

You’ll now need to wrap module augmentation of the JSX namespace in \`declare module ”…”:

```
// global.d.ts

+ declare module "react" {

    namespace JSX {

      interface IntrinsicElements {

        "my-element": {

          myElementProps: string;

        };

      }

    }

+ }
```

The exact module specifier depends on the JSX runtime you specified in the `compilerOptions` of your `tsconfig.json`:

- For `"jsx": "react-jsx"` it would be `react/jsx-runtime`.
- For `"jsx": "react-jsxdev"` it would be `react/jsx-dev-runtime`.
- For `"jsx": "react"` and `"jsx": "preserve"` it would be `react`.

### Better `useReducer` typings [Link for this heading](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#better-usereducer-typings "Link for this heading")

`useReducer` now has improved type inference thanks to [@mfp22](https://github.com/mfp22).

However, this required a breaking change where `useReducer` doesn’t accept the full reducer type as a type parameter but instead either needs none (and rely on contextual typing) or needs both the state and action type.

The new best practice is _not_ to pass type arguments to `useReducer`.

```
- useReducer<React.Reducer<State, Action>>(reducer)

+ useReducer(reducer)
```

This may not work in edge cases where you can explicitly type the state and action, by passing in the `Action` in a tuple:

```
- useReducer<React.Reducer<State, Action>>(reducer)

+ useReducer<State, [Action]>(reducer)
```

If you define the reducer inline, we encourage to annotate the function parameters instead:

```
- useReducer<React.Reducer<State, Action>>((state, action) => state)

+ useReducer((state: State, action: Action) => state)
```

This is also what you’d also have to do if you move the reducer outside of the `useReducer` call:

```
const reducer = (state: State, action: Action) => state;
```

## Changelog [Link for Changelog ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#changelog "Link for Changelog ")

### Other breaking changes [Link for Other breaking changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#other-breaking-changes "Link for Other breaking changes ")

- **react-dom**: Error for javascript URLs in `src` and `href` [#26507](https://github.com/facebook/react/pull/26507)
- **react-dom**: Remove `errorInfo.digest` from `onRecoverableError` [#28222](https://github.com/facebook/react/pull/28222)
- **react-dom**: Remove `unstable_flushControlled` [#26397](https://github.com/facebook/react/pull/26397)
- **react-dom**: Remove `unstable_createEventHandle` [#28271](https://github.com/facebook/react/pull/28271)
- **react-dom**: Remove `unstable_renderSubtreeIntoContainer` [#28271](https://github.com/facebook/react/pull/28271)
- **react-dom**: Remove `unstable_runWithPriority` [#28271](https://github.com/facebook/react/pull/28271)
- **react-is**: Remove deprecated methods from `react-is` [28224](https://github.com/facebook/react/pull/28224)

### Other notable changes [Link for Other notable changes ](https://react.dev/blog/2024/04/25/react-19-upgrade-guide\#other-notable-changes "Link for Other notable changes ")

- **react**: Batch sync, default and continuous lanes [#25700](https://github.com/facebook/react/pull/25700)
- **react**: Don’t prerender siblings of suspended component [#26380](https://github.com/facebook/react/pull/26380)
- **react**: Detect infinite update loops caused by render phase updates [#26625](https://github.com/facebook/react/pull/26625)
- **react-dom**: Transitions in popstate are now synchronous [#26025](https://github.com/facebook/react/pull/26025)
- **react-dom**: Remove layout effect warning during SSR [#26395](https://github.com/facebook/react/pull/26395)
- **react-dom**: Warn and don’t set empty string for src/href (except anchor tags) [#28124](https://github.com/facebook/react/pull/28124)

For a full list of changes, please see the [Changelog](https://github.com/facebook/react/blob/main/CHANGELOG.md#1900-december-5-2024).

* * *

Thanks to [Andrew Clark](https://twitter.com/acdlite), [Eli White](https://twitter.com/Eli_White), [Jack Pope](https://github.com/jackpope), [Jan Kassens](https://github.com/kassens), [Josh Story](https://twitter.com/joshcstory), [Matt Carroll](https://twitter.com/mattcarrollcode), [Noah Lemen](https://twitter.com/noahlemen), [Sophie Alpert](https://twitter.com/sophiebits), and [Sebastian Silbermann](https://twitter.com/sebsilbermann) for reviewing and editing this post.

[PreviousReact 19 RC](https://react.dev/blog/2024/04/25/react-19) [NextReact Labs: What We've Been Working On – February 2024](https://react.dev/blog/2024/02/15/react-labs-what-we-have-been-working-on-february-2024)

* * *

[Meta Open Source](https://opensource.fb.com/)

Copyright © Meta Platforms, Inc

no uwu plz

uwu?

Logo by [@sawaratsuki1004](https://twitter.com/sawaratsuki1004)

[Learn React](https://react.dev/learn)

[Quick Start](https://react.dev/learn)

[Installation](https://react.dev/learn/installation)

[Describing the UI](https://react.dev/learn/describing-the-ui)

[Adding Interactivity](https://react.dev/learn/adding-interactivity)

[Managing State](https://react.dev/learn/managing-state)

[Escape Hatches](https://react.dev/learn/escape-hatches)

[API Reference](https://react.dev/reference/react)

[React APIs](https://react.dev/reference/react)

[React DOM APIs](https://react.dev/reference/react-dom)

[Community](https://react.dev/community)

[Code of Conduct](https://github.com/facebook/react/blob/main/CODE_OF_CONDUCT.md)

[Meet the Team](https://react.dev/community/team)

[Docs Contributors](https://react.dev/community/docs-contributors)

[Acknowledgements](https://react.dev/community/acknowledgements)

More

[Blog](https://react.dev/blog)

[React Native](https://reactnative.dev/)

[Privacy](https://opensource.facebook.com/legal/privacy)

[Terms](https://opensource.fb.com/legal/terms/)

[React on Facebook](https://www.facebook.com/react)[React on Twitter](https://twitter.com/reactjs)[React on Bluesky](https://bsky.app/profile/react.dev)[React on Github](https://github.com/facebook/react)