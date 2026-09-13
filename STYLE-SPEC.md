# QuarkLang 通用样式规范 v2（Style 库契约：颗粒度全集 + 动画基础）

> **契约位置**：权威副本在 `QuarkLangLibs-Style/STYLE-SPEC.md`；cleg 仓库保留一份同步副本。
> 本文件是**通用样式库**（`QuarkLangLibs-Style/style.qk`）与 cleg 共用的样式键唯一权威清单（与实现同步维护）。
> **cleg 副本**：契约在 `QuarkLangLibs-Style/STYLE-SPEC.md`（通用样式库与 cleg 共用同一份；修改请同步两边）。
> 数据模型不变：`style HashTable<String,String>`；本规范只**扩充键**与**求值语义**，不引入第二套语法。
> 目标：对齐 Qt QSS 的颗粒度；动画按"最基础可用"落地（transition + animation + transform + opacity）。

## 0. 求值顺序（级联）

```
全局样式（Theme 实例：theme.apply(node) 写入节点 "__g~" 层）
  < 祖先节点样式（含选择器块）
  < 自身样式
  < 状态覆盖（键后缀 ":<state>"）
  < 动画覆盖（animation-* 在 t 时刻的插值）
```
每层内部：同一键后来者覆盖前者；`setStyle(jsonText)` 为**合并**（不整表替换）。

## 1. 盒模型（全部四边展开）

| 键 | 值 | 说明 |
|---|---|---|
| `margin` / `margin-top/right/bottom/left` | 长度 / "a,b" / "a,b,c,d"（上右下左） | 外边距 |
| `padding` / `padding-*` | 同上 | 内边距 |
| `border-width` / `border-*-width` | 长度（简写 1..4 值） | 边框宽（0 = 无边框） |
| `border-style` / `border-*-style` | none/solid/dashed/dotted | 边框样式（基础实现：solid/dashed/dotted） |
| `border-color` / `border-*-color` | 颜色 | 边框色 |
| `border-radius` / `border-{top-left,top-right,bottom-right,bottom-left}-radius` | 长度 / "a,b" | 圆角 |
| `box-sizing` | content/border（默认 border） | w/h 是否含 border+padding |
| `outline-width/style/color` | 同 border | 焦点外框（不占布局） |

**派生几何**：`contentRect = (x + margin + border + padding, y + …, w - …, h - …)`。
所有组件 render **先算 contentRect 再画**（C2）。

## 2. 背景

| 键 | 值 | 说明 |
|---|---|---|
| `background` / `background-color` | 颜色 | 背景色（`bg` 为旧别名，保留读取） |
| `background-gradient` | `linear(angle, c1, c1pos, c2, c2pos) | radial(cx,cy,r, c1, c2)` | 渐变（基础：linear 2 色 + radial 2 色） |
| `background-image` | 路径（png） | v2 基础：no-repeat 平铺原点 |
| `background-repeat` | no-repeat/repeat/repeat-x/repeat-y | v2 |
| `background-position` | `x y`（关键词或长度） | v2 |
| `alternate-background-color` | 颜色 | 列表/表格交替行 |
| `opacity` | 0..1 | 整节点透明度（alpha 混合） |

## 3. 边框与阴影（渲染原语见 §9）

| 键 | 值 | 说明 |
|---|---|---|
| `border-color` 等 | 见 §1 | 四边可分色 |
| `box-shadow` | `dx dy blur spread color [inset]`，可逗号多层 | 基础：单层/多层，blur 用均值近似 |

## 4. 文本

| 键 | 值 | 说明 |
|---|---|---|
| `color` | 颜色 | 文本色 |
| `font-family` | 回退链 `"A,B" ` | 已有 |
| `font-size` | px / pt（96dpi 折 px） | 已有 + pt |
| `font-style` | normal/italic | 变体选择 |
| `font-weight` | normal/bold/100..900 | 变体选择（bold 优先） |
| `font` | 简写 `bold italic 12px "A"` | 解析为上述三键 |
| `letter-spacing` | 长度（可负） | 字距 |
| `line-height` | 长度 / 倍数（1.4） | 行高 |
| `text-align` | left/center/right | 需文本度量（§9） |
| `text-valign` | top/middle/bottom | 同上 |
| `text-decoration` | none/underline/line-through | 画线 |
| `text-overflow` | clip/ellipsis | 超宽省略号 |
| `white-space` | normal/nowrap | 换行策略 |
| `placeholder-text-color` | 颜色 | 输入类占位符 |
| `echo-mode` | normal/password | 输入类回显（`password-char`） |

## 5. 几何 / 布局 / 可见性

| 键 | 值 | 说明 |
|---|---|---|
| `pos` / `geometry` | `"x,y"` / `"x,y,w,h"` | 位置（旧键保留） |
| `width`/`height`、`min-width`/`min-height`、`max-width`/`max-height` | 长度 | 尺寸约束 |
| `size-hint` | `"w,h"` | 推荐尺寸（布局用） |
| `size-policy` | `"h,v"`：fixed/minimum/maximum/preferred/expanding/ignored | 两轴伸缩策略 |
| `stretch` | int（默认 0） | 弹性因子 |
| `spacing` / `gap` | 长度 | 布局间距（`cleglayout` 读取） |
| `layout` | vbox/hbox/grid/form | 建议布局（`cleglayout` 读取） |
| `position` | static/relative/absolute | absolute 用 `top/left/right/bottom` |
| `top/left/right/bottom` | 长度 | 定位偏移 |
| `z-index` | int | 绘制与命中顺序 |
| `overflow` / `clip` | visible/hidden | 裁剪到 border rect |
| `visible` | true/false | false = 跳过 render 与命中 |
| `enabled` | true/false | false = 派生 `:disabled` 状态 |

## 6. 状态（伪状态 → 键后缀）

状态位由运行库维护：`cleg::setState(node, "hover", true)`；事件层（`cleg::dispatchEvent`）自动置位。

| 状态 | 键后缀示例 | 说明 |
|---|---|---|
| hover | `background-color:hover` | 悬停 |
| pressed | `background-color:pressed` | 按下 |
| checked / unchecked | `background-color:checked` | 勾选（组件字段同步并入状态位） |
| disabled / enabled | `color:disabled` | 禁用 |
| focus | `outline-color:focus` | 焦点 |
| selected | `background-color:selected` | 选中（列表/表格/菜单/tab） |
| alternate | `:alternate` | 交替行 |
| open / closed | `:open` | 展开/收起 |
| on / off | `:on` | 开关 |
| indeterminate | `:indeterminate` | 半选 |
| read-only | `:read-only` | 只读 |
| flat / default | `:flat` / `:default` | 按钮类 |
| active | `:active` | 活动窗口 |

**多状态**：`键:hover:pressed`（AND，按后缀顺序无关；求值时从最具体到最宽泛回退）。
**选择器块**（C10/E1 基础版）：`setStyle` 的 JSON 支持顶层选择器键：
`{"*": {...}, "ClegButton": {...}, "ClegButton#ok": {...}, "ClegButton:hover": {...}, "ClegButton::chunk": {...}}`，
按 CSS2 特异性（ID > 类型/伪态 > 通用）与"后出现优先"合并；`#id` 匹配 `style["object-name"]`。

## 7. 动画（最基础可用集）

| 键 | 值 | 说明 |
|---|---|---|
| `transition` | `property duration timing delay[, …]` | 属性变化时补间（如 `opacity 200ms ease-out`） |
| `transition-property` / `-duration` / `-timing-function` / `-delay` | 展开键 | 同上 |
| `animation` | `name duration timing delay iteration-count direction fill-mode[, …]` | 关键帧动画 |
| `animation-name` / `-duration` / `-timing-function` / `-delay` / `-iteration-count` / `-direction` / `-fill-mode` / `-play-state` | 展开键 | 同上 |
| `transform` | `translate(dx,dy) scale(sx[,sy]) rotate(deg)` | 基础：translate/scale（rotate 渐进） |
| `transform-origin` | `x y` / 关键词 | 变换原点（scale 用） |

**可插值属性**：长度类（margin/padding/border-width/width/height/left/top/…）、颜色（color/background-color/border-color/…）、数值（opacity/z-index/letter-spacing/line-height）、`transform`。
**缓动**：`linear`、`ease`、`ease-in`、`ease-out`、`ease-in-out`、`cubic-bezier(a,b,c,d)`、`steps(n)`。
**关键帧**：实例化注册，无模块级全局：
```qk
Animator an = Animator::new();   // QuarkLangLibs-Style
an.define("pulse", "{\"0\":{\"opacity\":\"1\",\"transform\":\"scale(1)\"},\"0.5\":{\"opacity\":\"0.4\",\"transform\":\"scale(1.2)\"},\"1\":{\"opacity\":\"1\",\"transform\":\"scale(1)\"}}");
an.tick(nodes, 16);   // 每帧推进：写入各节点的动画覆盖层
```
**求值**：动画覆盖层按 §0 顺序最高优先；只覆盖声明过的键；`animation-play-state: paused` 时不推进。
**语义**：`iteration-count: n|infinite`；`direction: normal|reverse|alternate`；`fill-mode: none|forwards|backwards|both`。

## 8. 组件接入顺序（优先级）

1. 基类/窗口：ClegWindow/ClegMainWindow（背景/边框/圆角/阴影/opacity）
2. 文本类：ClegLabel、ClegLineEdit（文本对齐/装饰/占位符/盒模型）
3. 按钮类：ClegButton、ClegCheckBox、ClegRadioButton（状态：hover/pressed/checked/disabled/focus）
4. 数值类：ClegProgressBar、ClegSlider、ClegScrollBar（`::chunk`/`::groove`/`::handle` 子件键）
5. 数据类：ClegListView、ClegTableView、ClegTreeView（selection/alternate/gridline/表头 section）
6. 菜单/工具栏：ClegMenu、ClegMenuBar、ClegToolBar（item/separator/indicator 状态键）
7. 其余组件：套用同一套盒模型/背景/文本/状态求值路径

## 9. 渲染原语（native 必须提供，命名即 C ABI 导出）

**冻结签名（qk 侧 `library clegrt` 声明与 native 导出必须逐字一致）**：

```c
int  cleg_blend_rect(int x, int y, int w, int h, unsigned int argb, int alpha, int radius);
int  cleg_gradient(int x, int y, int w, int h, int dir, unsigned int c1, unsigned int c2, int radius);
int  cleg_radial(int cx, int cy, int r, unsigned int c1, unsigned int c2);
int  cleg_border(int x, int y, int w, int h, int wt, int wr, int wb, int wl,
                 unsigned int ct, unsigned int cr, unsigned int cb, unsigned int cl,
                 int radius, int style);            /* style: 0=solid 1=dashed 2=dotted */
int  cleg_shadow(int x, int y, int w, int h, int dx, int dy, int blur, int spread,
                 unsigned int argb, int inset, int radius);
int  cleg_clip_push(int x, int y, int w, int h);
int  cleg_clip_pop(void);
int  cleg_text_width(char* text, int size, char* font);
int  cleg_text_height(int size);
int  cleg_text_ex(char* text, int x, int y, int size, char* font, unsigned int color,
                  int align, int valign, int letter_spacing, int line_height,
                  int decoration, int ellipsis, int max_w);
                  /* align 0=left 1=center 2=right；valign 0=top 1=middle 2=bottom；
                     decoration 0=none 1=underline 2=line-through；ellipsis 0/1 */
int  cleg_image(char* path, int x, int y, int w, int h, int repeat); /* repeat 0=no-repeat 1=repeat 2=repeat-x 3=repeat-y */
int  cleg_transform(int dx, int dy, int sx_num, int sx_den, int sy_num, int sy_den); /* 基础变换（有理缩放，避免浮点 ABI） */
```
（`argb`/`c1`/`c2` 均为 0xAARRGGBB；`cleg_text_ex` 需兼容 CJK 回退链。）

（现有导出 `cleg_create/clear/rect/roundrect/text/frame/screen_open/present/close` 保持兼容。）

## 10. 验收（示例 + 帧验证）

1. `examples/style-showcase.qk`：一屏展示盒模型/四边边框/圆角/渐变/阴影/文本对齐/状态覆盖 → 输出 PNG。
2. `examples/animation.qk`：opacity + translate + scale + 颜色过渡的关键帧动画，`tick` 在 t=0/250/500/750/1000ms 各渲染一帧 → 5 张 PNG，帧间必须有可见差异（像素校验）。
3. `tests/`：qss 解析单测（长度简写/颜色/渐变/transform/缓动）、动画插值单测（线性/缓动端点）、状态回退单测。

### 9.1 语义裁定（native 已实现，qk 侧必须按此对齐）

| 项 | 裁定 |
|---|---|
| `cleg_gradient` 的 `dir` | 0/1/2/3 = 左→右 / 上→下 / 右→左 / 下→上；其它值按角度（0=左→右，90=上→下，顺时针） |
| `cleg_text_ex` 的 align/valign | **native 语义**：x = 参照盒左界（align 1/2 时在 [x, x+max_w] 内居中/右对齐，max_w<=0 时参照 = 最宽行）；valign 0/1/2 → y = 文本块顶 / 垂直中心 / 底。
| qk 侧传参约定（v2 修正） | align 0/1/2 一律传 **内容盒左界** + `max_w = 内容盒宽`（由 native 完成居中/右对齐）；只有内容盒宽<=0 时才自行按 `cleg_text_width` 平移锚点。旧实现自加 `w/2` 与 native 居中叠加 → 居中文字右移半个盒宽并溢出（海洋示例即此缺陷） |
| `line_height` | 单位 px；<=0 → 默认 size+4（= cleg_text_height）；倍数（1.4）由 qk 侧换算 |
| `max_w` | <=0 = 不限宽 |
| alpha 约定 | `blend_rect` 的 argb A==0 视为 255（透明度看 alpha 参数）；gradient/radial 两端 A 同时 0 = 不透明，否则按原义（可一端 0x00 渐隐）；shadow/border/text_ex 的 A==0 视为 255。要"完全透明"用 A=1..254 或 blend_rect 的 alpha |
| 度量 | `cleg_text_width/height` 不需 cleg_create、不受 transform 影响（多行取最宽行） |
| 返回码 | 成功 0；未 create = -1；clip_pop 空栈 = -1；image 解码失败 = -2 |
| transform | 绝对设置 device = p*s + t（非级联）；sx_den/sy_den 为 0 当 1；sy 两参数均为 0 → sy=sx；全 0 = 单位变换；**C ABI 无 rotate** |
| `blend_rect` radius | <=0 视作直角 |

**qk 侧约定**：颜色统一输出 `0xAARRGGBB`（A 用 0xFF 或真实透明度），避免 A==0 的兼容推断歧义。
**已知限制（v1 既有，非本次引入）**：TTF 光栅器个别字形轮廓填充缺失（"测得宽、画得稀"）；5×7 兜底表有空洞。若要文字清晰需另开任务修光栅器。

## 11. 通用库结构（QuarkLangLibs-Style/style.qk）

```qk
import "style";                 // 纯 qk、零依赖、零 FFI

// 节点契约（结构化满足，无需声明实现）
type interface {
    fn getStyle(Self self) HashTable<String, String>;
    fn setStyle(Self self, String jsonText) void;
    fn getX(Self self) int;  fn getY(Self self) int;
    fn getW(Self self) int;  fn getH(Self self) int;
} StyleNode;

// 值解析（纯计算）
qss::len / box4 / color / cR|cG|cB|cA / argbStr / gradient / gradDir / transform / ease
qss::time / transitions / animationParts / frames / frameTime / frameProps / propGet
qss::interpValue / interpColor / sameValue / extractBlock / jsonPairs

// 级联求值 / 盒模型 / 状态
style::get / getD / getI / getF / getSub / subColor / getBase / getThemed
style::stateOf / setState / hasState / stateHas / normalizeKey / setStyle
style::contentRect / borderRect / visible / isClipped / radiusOf / borderStyle / borderColor
style::globalTo / clearGlobal / inherit

// 主题（实例化全局层；无文件、无模块级全局）
Theme t = Theme::fromJson("{"color":"#e6edf7"}");
t.set("font-size", "14");   t.get("font-size");   t.apply(node);   t.applyAny(anyNode);
style::getThemed(t, node, "color", "")

// 动画（状态在实例内）
Animator an = Animator::new();
an.define("pulse", "{"0":{"opacity":"1"},"1":{"opacity":"0.4"}}");
an.tick(nodes, 16);
```

**节点桥接**：qk 的接口→接口赋值按名字严格判定，若调用方持有的是另一个接口类型（如 cleg 的 `ClegNode`），
先赋给 `interface{}` 再传入即可（运行时是同一节点值）：

```qk
interface{} anyNode = clegNode;
style::get(anyNode, "color", "");
```
