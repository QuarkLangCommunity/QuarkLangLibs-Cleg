# QuarkLangLibs-Cleg

**QuarkLang 官方库（cleg）——全面 GUI 框架（Qt 参考），官方认证。**

> ⚠️ **旧 style 键 `font`/`size` 已弃用**（仅 5×7 位图路径：小写残缺、字号格子化）。
> 统一使用 QSS 语义键：`font-family`（字体回退链，含 **CJK 自动选择**）+ `font-size`（像素，FreeType 全字符）。

## 核心设计

- **ClegNode 核心接口**（dynamic 协议）：`render` / `setStyle(jsonText)` / `getStyle` / `getX` / `getY` / `getW` / `getH`
- **47 类组件**全部实现 ClegNode（window/标准容器/文本输入/按钮/数值状态/数据视图/菜单栏/日期时间/其它，见下）
- **全节点 Style（HashTable<String,String>）驱动渲染**，语义键采用 **QSS**（Qt Style Sheets）命名：
  ```qk
  label.style["text"] = "HELLO";
  label.style["color"] = "52,211,235";            // r,g,b
  label.style["background-color"] = "240,98,146";
  label.style["font-size"] = "46";                // px
  label.style["font-family"] = "Adwaita Sans, Liberation Sans, monospace";  // 字体回退链
  label.style["border-radius"] = "12";
  label.style["font-weight"] = "bold";            // v2
  label.style["selection-background-color"] = "52,211,235";  // 列表/表格选中
  label.style["selection-color"] = "8,28,46";
  ```
- **setStyle(string) 是真正入口**：string = **要解析的 JSON 字符串本体**（不是文件名）
  ```qk
  label.setStyle(`{"text": "HELLO", "color": "52,211,235", "font-size": "46", "font-family": "Liberation Sans, monospace"}`);
  ```
  **`@styleConfigure(file)` 给 setStyle 服务**：读 file → 文件内容字符串 → 交给 setStyle
  ```qk
  label.setStyle("") @styleConfigure(file::new("/tmp/label.json"));
  ```
- 字体回退链：`font-family` 逐名探测系统字体（Linux `/usr/share/fonts`、macOS `/System/Library/Fonts`、Windows `C:\Windows\Fonts`），FreeType 光栅（Linux）渲染，链末端内置 5×7 位图兜底——跨系统一致

## Style v2（颗粒度扩展 + 动画基础）

> 权威键清单见 **`STYLE-SPEC.md`**（键全集 / 级联顺序 / 状态 / 动画 / §9 冻结的 native 签名；契约权威副本在 `QuarkLangLibs-Style/STYLE-SPEC.md`）。
> **实现分层（已普适化）**：通用样式引擎在独立库 **`QuarkLangLibs-Style/style.qk`**（`space qss` 纯解析 → `space style` 级联/盒模型 → `Theme` 主题 → `Animator` 动画，纯 qk 零依赖零 FFI）；
> cleg 通过 `import "style"` 消费它，只保留 cleg 专属部分：`clegfx`（native 绘制封装）、`space clegstyle`（薄转发 + 组件配方）、47 组件与 `ClegAnimator`（`style::Animator` 的薄封装）。

### 级联（低 → 高）

```
构造默认 (__def~) < 全局样式 Theme.apply(node) < 祖先快照 cleg::inherit(child,parent)
  < 自身样式（含选择器块） < 状态覆盖 key:state < 动画/补间覆盖 __anim~
```

`clegstyle::get(node, key, state)` 按此求值（另有 `getBase` 跳过动画层、`getSub` 读子件）；`setStyle(jsonText)` 为**合并**（不整表替换）。
qk 无模块级全局：全局样式改为**实例化主题对象**（不再有 `/tmp/cleg-global-style.json`）——调用方持有 `Theme`，用 `t.apply(node)`（或 `cleg::applyTheme(t, node)`）写入节点全局层，`cleg::getThemed(t, node, key, state)` 可直接求值：
```qk
Theme t = cleg::theme("{\"color\": \"#e6edf7\", \"font-size\": \"14\"}");   // 或 Theme::fromJson(...)
cleg::applyTheme(t, win);                        // 全局层
t.set("font-size", "16");                        // 主题可变，rev 变化后 apply 自动重写
cleg::getThemed(t, label, "color", "");          // 或先 apply 再 clegstyle::get(label, "color", "")
```
`cleg::clearTheme(node)` / `cleg::refreshStyle(node)` 失效该节点的全局层（下次 apply 重写）。祖先层仍由 `cleg::inherit(child, parent)` 显式快照。

### 键覆盖（已生效）

| 组 | 键 |
|---|---|
| §1 盒模型 | `margin(-top/right/bottom/left)` `padding-*` `border-width(-*)`（1..4 值 `"a,b,c,d"` = 上右下左） `border-style(-*)`（solid/dashed/dotted） `border-color(-*)` `border-radius(-*)` `box-sizing` `outline-width/style/color` |
| §2 背景 | `background(-color)` `background-gradient`（`linear(angle,c1,p1,c2,p2)` / `radial(cx,cy,r,c1,c2)`） `background-image` `background-repeat` `background-position` `opacity` `alternate-background-color` |
| §3 阴影 | `box-shadow`（`dx dy blur spread color [inset]`，逗号多层） |
| §4 文本 | `color` `font-family` `font-size`（px/pt） `font-style` `font-weight` `font`（简写） `letter-spacing` `line-height`（px 或倍数） `text-align` `text-valign` `text-decoration` `text-overflow: ellipsis` `white-space` `placeholder-text-color` `echo-mode`/`password-char` |
| §5 几何 | `pos` `visible` `overflow`/`clip`（裁剪到 border rect） `transform`（translate/scale） `gap`/`spacing`（`cleglayout` 读取） |
| §6 状态 | 任意 `key:state`（多状态 `key:hover:pressed`，后缀顺序无关；最具体→最宽泛回退）、`enabled=false` 派生 `:disabled` |
| §7 动画 | `transition`（+展开键）、`animation`（+展开键 name/duration/timing-function/delay/iteration-count/direction/fill-mode/play-state）、`transform` |
| 子件 | `key::chunk` `::groove` `::handle` `::indicator` `::gridline`（含选择器式 `Type::chunk { ... }`） |

颜色：`#rgb` `#rrggbb` `#aarrggbb` `rgb()` `rgba()` 具名色（red/steelblue/…）与旧式 `r,g,b`。
缓动：`linear` `ease` `ease-in` `ease-out` `ease-in-out` `cubic-bezier(a,b,c,d)` `steps(n[,start])`。

### 选择器块（CSS2 特异性）

```qk
b.setStyle(`{"*": {"color": "#8899aa"},
             "ClegButton": {"background-color": "#2f6f4f"},
             "ClegButton#ok": {"background-color": "#7a4fd0"},        // #id 匹配 style["object-name"]
             "ClegButton:hover": {"background-color": "#ffb020"},
             "ClegButton::chunk": {"background-color": "#123456"}}`);
```

ID(100) > 类型(1)/伪状态(10) > 通用(0)；同分"后出现优先"。选择器块存于 style 表的 `__sel~…` 保留键。

### 状态位（§6）

```qk
cleg::setState(btn, "hover", true);              // → style["__state:hover"]
cleg::stateOf(btn);                              // "enabled:hover"
clegsignal::dispatchEvent(btn, "press", true);   // 事件 → 状态位（hover/pressed/focus/checked/…）+ 信号
```

组件字段并入状态：`ClegCheckBox/ClegRadioButton::setChecked` 同步 `checked/unchecked`。

### 动画（§7）

```qk
ClegAnimator an = ClegAnimator::new();   // style::Animator 的薄封装；状态全部在实例内（无模块级全局）
an.define("pulse", `{"0":  {"opacity": "1",    "transform": "translate(0,0) scale(1)",      "background-color": "#34d3eb"},
                      "0.5":{"opacity": "0.35", "transform": "translate(220,120) scale(1.5)", "background-color": "#ff5f6d"},
                      "1":  {"opacity": "1",    "transform": "translate(0,0) scale(1)",      "background-color": "#34d3eb"}}`);
node.getStyle()["animation"] = "pulse 1000ms ease-in-out 0ms 1 normal both";
an.tick(nodes, 16);                      // 每帧推进 → 写 __anim~ 覆盖层；只覆盖声明过的键
```

可插值：长度 / 颜色 / 数值（opacity/line-height…）/ `transform`；`transition` 在属性变化时自动补间（起点 = 上一目标值或在飞插值）。
`iteration-count: n|infinite`、`direction: normal|reverse|alternate`、`fill-mode: none|forwards|backwards|both`、`animation-play-state: paused` 均已实现。

### 渲染原语（§9，native `bin/libclegrt.so`）

`cleg_blend_rect` / `cleg_gradient` / `cleg_radial` / `cleg_border` / `cleg_shadow` / `cleg_clip_push` / `cleg_clip_pop` / `cleg_text_width` / `cleg_text_height` / `cleg_text_ex` / `cleg_image` / `cleg_transform`（签名与语义裁定见 `STYLE-SPEC.md` §9/§9.1）。
- `dir`：0/1/2/3 = 左→右/上→下/右→左/下→上；其它值按角度（0=左→右，90=上→下）
- `text_ex` 的 align/valign：**x = 参照盒左界**（align 1/2 在 `[x, x+max_w]` 内居中/右对齐，max_w<=0 时参照最宽行），**y = 锚点**（valign 1/2 = 盒垂直中心/盒底）；
  qk 侧 `paintTextC` 一律传「内容盒左界 + max_w=内容盒宽」（v2 修正：旧实现自加 `w/2` 与 native 居中叠加 → 居中文字右移半盒并溢出色块边界）
- 颜色统一 `0xAARRGGBB`（A=0xFF 或真实 alpha）；`cleg_transform` 为绝对设置（device = p*s + t）、**无 rotate**

### 示例 / 测试

```bash
QUARK=/tmp/quark ./tests/run.sh      # cleg.qk 类型检查 + qss 解析 / 动画插值 / 状态回退 / 47 组件扫掠 / Style 特性
QUARK=/tmp/quark ./examples/run.sh   # style-showcase.png + anim-{0,250,500,750,1000}.png（5 帧 md5 互异校验）
STYLE_LIB=/path/style.qk QUARK=/tmp/quark ./tests/run.sh     # 通用样式库不在 ../QuarkLangLibs-Style 时显式指定
```

两个脚本会把 `cleg.qk` + `style.qk` + `json.qk` + `libclegrt.so` 复制到临时目录运行（同目录解析 `import` 与 `dlopen`）；
`style.qk` 可用 `STYLE_LIB=/path/style.qk` 指定（默认找 `../QuarkLangLibs-Style/style.qk`），`json.qk` 可用 `JSON_LIB=/path/json.qk` 指定。
**类型改名**：通用类型随库迁移并去掉 cleg 前缀——`ClegGradient`→`StyleGradient`、`ClegTransform`→`StyleTransform`（`Rect` 不变）；`qss::`/`clegstyle::`/`cleg::` 等 API 名称保持兼容。

### 未接入 / 限制（如实报告）

- `rotate(...)` 已解析并参与插值，但 §9 C ABI 无 rotate → **不渲染旋转**（丢弃）；`transform-origin` 未生效（scale 以设备原点为原点）。
- 未接入：`z-index`（绘制顺序仍为列表顺序）、`size-hint`/`size-policy`/`stretch`/`layout`（布局建议键）、`::section`（表头子件）、菜单/工具栏 `item/separator` 子件键；`font-weight/font-style` 已解析但 TTF 变体选择未实现（沿用族名）。
- 已知 native 限制（非本次引入）：TTF 光栅器个别字形轮廓填充缺失（"测得宽、画得稀"）、5×7 兜底表有空洞；样式/动画验收以几何与颜色为准。

## 组件全集（47 类，全部实现 ClegNode 接口）

- **窗口/容器**：ClegMainWindow（默认窗口）、ClegDialog、ClegFrame、ClegGroupBox、ClegScrollArea、ClegSplitter、ClegDockWidget、ClegMdiArea、ClegMdiSubWindow、ClegStackedWidget、ClegTabWidget、ClegToolBox
- **文本输入/显示**：ClegLabel、ClegLineEdit、ClegTextEdit、ClegPlainTextEdit、ClegTextBrowser
- **按钮类**：ClegButton（=QPushButton）、ClegCheckBox、ClegRadioButton、ClegDialogButtonBox
- **数值/状态**：ClegProgressBar、ClegSlider、ClegScrollBar、ClegDial、ClegSpinBox、ClegDoubleSpinBox、ClegLCDNumber
- **数据视图**：ClegComboBox、ClegListWidget、ClegListView、ClegTableWidget、ClegTableView、ClegTreeWidget、ClegTreeView、ClegColumnView、ClegHeaderView
- **菜单/工具栏**：ClegMenuBar、ClegMenu、ClegToolBar、ClegStatusBar
- **日历/时间**：ClegCalendarWidget、ClegDateEdit、ClegTimeEdit
- **其它**：ClegSizeGrip、ClegOpenGLWidget、ClegVideoWidget

每类统一：`type struct { ... } Name;` + `impl { ... } Name;`（结构化满足 ClegNode 接口；定制 render / getStyle / getX / getY / getW / getH / new）+ 状态字段 + 专有方法（`setChecked` / `setValue` / `currentIndex` / `addItem` / `setPlainText` / `display` …）+ `setStyle(jsonText)`。

## 事件接口族（组件协议：用户直接用类实现）

内置接口（dynamic，`qksignal_emit` 派发，有则调用、无则忽略）。**事件接口族是"可选实现"**：实现多少方法随意（只写 onClicked 即可，缺 onPressed/onReleased 不报错）；`emit` 只触发已实现的方法。**同一类型只能有一个 `impl { ... } Name;` 块**（正典语法不再用接口名区分 impl），协议方法写在该类型的 impl 内。

`ClegClickable`（onClicked/onPressed/onReleased）· `ClegCheckable`（onToggled）· `ClegEditable`（onTextChanged/onReturnPressed）· `ClegValueable`（onValueChanged）· `ClegSelectable`（onCurrentIndexChanged）· `ClegItemable`（onItemClicked）· `ClegCellable`（onCellClicked）· `ClegCloseable`（onCloseRequested）· `ClegActionable`（onTriggered）

```qk
// 自定义类：一个 impl 内写齐所需方法（接口不写在 impl 上，方法齐全即结构化满足）
type struct { HashTable<String, String> style; int x; int y; int w; int h; } MyWatcher;

impl {
    fn new() MyWatcher { ... }
    // ClegNode 协议（结构化满足，方法齐全即可）
    fn render(MyWatcher self) void { ... }
    fn setStyle(MyWatcher self, String jsonText) void { ... }
    fn getStyle(MyWatcher self) HashTable<String, String> { return self.style; }
    fn getX(MyWatcher self) int { return self.x; }
    fn getY(MyWatcher self) int { return self.y; }
    fn getW(MyWatcher self) int { return self.w; }
    fn getH(MyWatcher self) int { return self.h; }
    // 事件协议方法（ClegClickable/ClegSelectable 等，有则被 emit 调用）
    fn onClicked(MyWatcher self) void { ... }
    fn onCurrentIndexChanged(MyWatcher self, int idx) void { ... }
} MyWatcher;

clegsignal::emit(b, "clicked");                        // 无参——短名自动映射 onClicked（全名也可：emit(b,"onClicked")）
clegsignal::emitArgs(w, "onCurrentIndexChanged", 42);  // 带参（int）
clegsignal::emitText(w, "onTextChanged", "HELLO");     // 带参（String）
```

## 布局 / 信号

```qk
List<ClegNode> items = List::new();
items.append(label); items.append(btn); items.append(slider);
cleglayout::vbox(items, 40, 60, 300, 40);   // vbox / hbox / grid（Qt 布局语义：重排节点 style["pos"]）

clegsignal::emit(btn, "clicked");           // 触发节点 onClicked 方法（有则调用）
```

## 渲染后端

软件光栅帧缓冲（线性 u32、预分配、零分配热路径；4K 全屏 2.5ms）：
- **Style v2 原语（native C ABI，qk 侧 `library clegrt` 声明）**：`cleg_blend_rect` / `cleg_gradient` / `cleg_radial` / `cleg_border` / `cleg_shadow` / `cleg_clip_push` / `cleg_clip_pop` / `cleg_text_width` / `cleg_text_height` / `cleg_text_ex` / `cleg_image` / `cleg_transform`（+ 旧 `cleg_create/clear/rect/roundrect/text/frame/screen_*` 保持兼容）
- 运行时内建：`qksignal_emit`、`qkfile_read/write`、`qkjson_dumps/loads`
- 屏幕宿主跨系统：Linux=X11（实测窗口弹出）、Windows=GDI（交叉编译产出 PE32+）、其它=PNG 帧输出
- `cleg::show(w,h,title)` / `cleg::present()` / `cleg::hide()`（`cleg::init/frame` 帧输出模式）

## Qt 对齐路线

官方研读差距表见 **`QT-QSS-to-CLEG-GAP.md`**（Qt 6.11 官方文档核对：QSS 属性全集 ~100 键、45 伪状态、36 子控件、191 类清单、信号槽模型；cleg 差距 C1–C13 必有 / D1–D19 应有 / E1–E10 可选）。

已落地：QSS 键（color/background-color/font-size/font-family/border-radius/selection-*）、字体回退链、圆角、布局（vbox/hbox/grid + `gap`/`spacing`）、信号（emit→onXxx）、选中态、接口泛型（List<ClegNode>）。
**Style v2 已落地**（见上节）：四边盒模型 / 渐变 / 阴影 / 四边边框 / 文本对齐·装饰·省略号 / 状态覆盖与 `:disabled` 派生 / 选择器块（CSS2 特异性）/ 全局与祖先层 / `transition` 补间 / 关键帧动画 `ClegAnimator`；47 组件 render 全部走统一盒模型与背景路径，其中窗口/文本/按钮/数值四组（§8 优先级 1–4）走完整配方（背景 + 四边边框 + 文本 + 子件）。
**通用化重构**：样式引擎（`qss` + 级联/盒模型 + `Theme` + `Animator`）已抽到独立库 `QuarkLangLibs-Style/style.qk`（纯 qk、零依赖、零 FFI、无模块级全局），cleg 仅保留 native 绘制与组件层并保持既有 API（`clegstyle::get` 等为薄转发）。`cleg.qk` 因此从 4980 行降到约 3430 行。

## 认证信息

- 库名：`cleg`（`import "cleg"`）
- 语言版本：QuarkLang v0.2（接口 dynamic 派发 / `expand interface X;` 语句 / 运算符重载 / 多 impl 聚合 / 函数重载）
- 运行时版本：见主仓库 `engineVersion`；光栅基准：fill4K 2.5ms、文本 58ns/字符
- 认证方：QuarkLang 官方项目

> 官方库=认证；光栅/字体/屏幕宿主在 QuarkLang 运行时内部；窗口系统（X11/GDI）宿主已实测（Linux）/交叉编译（Windows）/帧输出（其它）。
